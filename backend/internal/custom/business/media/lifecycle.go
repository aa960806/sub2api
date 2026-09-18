package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	mediaPhasePrepared    = "prepared"
	mediaPhaseReserving   = "reserving"
	mediaPhaseReserved    = "reserved"
	mediaPhaseSubmitting  = "submitting"
	mediaPhaseUnknown     = "submission_unknown"
	mediaPhaseSubmitted   = "submitted"
	mediaPhaseCapturing   = "capturing"
	mediaPhaseReleasing   = "releasing"
	mediaPhaseSettled     = "settled"
	mediaPhaseReleased    = "released"
	mediaPhaseRejected    = "rejected"
	mediaPhaseReview      = "manual_review"
	mediaLeaseTTL         = 5 * time.Minute
	mediaOperationTimeout = 110 * time.Second
)

type mediaPriceQuoter interface {
	Quote(context.Context, string, *APIKey) (float64, error)
}

// SubmissionRejected is only returned for a structured, definitive upstream
// rejection. Transport errors and malformed/HTML responses are ambiguous.
type SubmissionRejected struct{ Cause error }

func (e *SubmissionRejected) Error() string { return e.Cause.Error() }
func (e *SubmissionRejected) Unwrap() error { return e.Cause }

func mediaDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func mediaAccountFingerprint(a *Account) string {
	return mediaDigest(a.GetSeedanceBaseURL() + "\x00" + a.GetSeedanceAPIKey())
}

func (h *MediaTaskHandler) prepareTask(ctx context.Context, key *APIKey, req *MediaVideoCreateRequest, idem string) (any, bool, error) {
	store, ok := h.tasks.Store().(DurableMediaTaskStore)
	quote, quoteOK := h.tasks.Billing().(mediaPriceQuoter)
	if !ok || !quoteOK {
		return nil, false, ErrMediaTaskUnavailable
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return nil, false, err
	}
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(mediaCreateIdempotencyScope(key.UserID, key.ID)+"\x00"+idem)).String()
	hash := mediaDigest(string(encoded))
	// Replay precedes price lookup and reference resolution: changing a price or
	// expiring an uploaded image must not turn a retry into a second billable job.
	if existing, getErr := store.GetTask(ctx, id); getErr == nil {
		if existing.UserID != key.UserID || existing.APIKeyID != key.ID || existing.RequestHash != hash {
			return nil, false, ErrMediaIdempotencyConflict
		}
		return existing.PublicTask(existing.Phase == mediaPhaseSettled), true, nil
	} else if !errors.Is(getErr, ErrMediaTaskNotFound) {
		return nil, false, ErrMediaTaskUnavailable.WithCause(getErr)
	}
	price, err := quote.Quote(ctx, req.Model, key)
	if err != nil {
		return nil, false, err
	}
	images, pinned, err := h.tasks.ResolveFiles(ctx, req.FileIDs, key.UserID, key.ID)
	if err != nil {
		return nil, false, err
	}
	account, err := h.selectAccount(ctx, key, req.Model, pinned)
	if err != nil {
		return nil, false, err
	}
	if account.HasAnyQuotaLimit() {
		return nil, false, infraerrors.BadRequest("MEDIA_ACCOUNT_QUOTA_UNSUPPORTED", "Seedance account budget limits require async reservations; use user balance and API key limits")
	}
	if account.GetMappedModel(req.Model) != req.Model {
		return nil, false, infraerrors.BadRequest("MEDIA_MODEL_MAPPING_UNSUPPORTED", "Seedance model remapping is not supported")
	}
	if account.RateMultiplier != nil && *account.RateMultiplier != 1 {
		return nil, false, infraerrors.BadRequest("MEDIA_MULTIPLIER_UNSUPPORTED", "Seedance account multiplier must be 1")
	}
	task := &MediaTaskRecord{
		ID: id, UserID: key.UserID, APIKeyID: key.ID, AccountID: account.ID, GroupID: key.GroupID,
		Platform: PlatformSeedance, Model: req.Model, DurationSeconds: req.DurationSeconds,
		Resolution: req.Resolution, Ratio: req.Ratio, CameraMovement: req.CameraMovement, FileIDs: req.FileIDs,
		Status: MediaTaskStatusQueued, Phase: mediaPhasePrepared, UnitPrice: price, CreatedAt: time.Now().Unix(),
		RequestHash: hash, Request: req, UpstreamImageIDs: images,
		UpstreamKey: "sub2-media-" + id, CredentialHash: mediaAccountFingerprint(account),
		BillingQuota: key.Quota > 0, BillingRateLimits: key.HasRateLimits(),
		BillingAccountQuota: account.HasAnyQuotaLimit(), BillingAccountType: account.Type,
	}
	task, created, err := store.CreateTask(ctx, task)
	if err != nil {
		return nil, false, err
	}
	// The intent is durable before funds move. Reserve is short and contains no
	// upstream call. A lost response can always be recovered with this task ID.
	if err := h.advanceTask(ctx, task.ID, true); err != nil {
		logger.L().Warn("media.reserve_pending", zap.String("task_id", task.ID), zap.Error(err))
	}
	if latest, err := store.GetTask(ctx, task.ID); err == nil {
		task = latest
	}
	return task.PublicTask(task.Phase == mediaPhaseSettled), !created, nil
}

func mediaBillingIdentity(task *MediaTaskRecord) (*APIKey, *Account) {
	key := &APIKey{ID: task.APIKeyID, UserID: task.UserID, GroupID: task.GroupID}
	if task.BillingQuota {
		key.Quota = 1
	}
	if task.BillingRateLimits {
		key.RateLimit5h = 1
	}
	return key, &Account{ID: task.AccountID, Platform: PlatformSeedance, Type: task.BillingAccountType}
}

func (h *MediaTaskHandler) boundAccount(ctx context.Context, task *MediaTaskRecord) (*Account, error) {
	account, err := h.gatewayService.GetMediaTaskAccount(ctx, task.AccountID)
	if err != nil || account == nil || !account.IsSeedance() || account.Type != service.AccountTypeAPIKey {
		return nil, ErrMediaAccountUnavailable
	}
	if mediaAccountFingerprint(account) != task.CredentialHash {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "MEDIA_ACCOUNT_CHANGED", "original upstream account credentials changed; reconciliation required")
	}
	return account, nil
}

func mediaFinalPhase(phase string) bool {
	return phase == mediaPhaseSettled || phase == mediaPhaseReleased || phase == mediaPhaseRejected || phase == mediaPhaseReview
}

// advanceTask serializes each intent with a fenced DB lease. A financial phase
// is persisted before the corresponding idempotent SQL operation, so a crash
// never changes a capture into a refund or vice versa.
func (h *MediaTaskHandler) advanceTask(parent context.Context, id string, reserveOnly bool) error {
	store, ok := h.tasks.Store().(DurableMediaTaskStore)
	if !ok || h.tasks.Billing() == nil {
		return ErrMediaTaskUnavailable
	}
	ctx, cancel := context.WithTimeout(parent, mediaOperationTimeout)
	defer cancel()
	token := uuid.NewString()
	acquired, err := store.AcquireTask(ctx, id, token, mediaLeaseTTL)
	if err != nil {
		return err
	}
	if !acquired {
		return nil
	}
	defer func() {
		releaseCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = store.ReleaseTask(releaseCtx, id, token)
	}()
	ctx = WithMediaLeaseContext(ctx, token)
	task, err := store.GetTask(ctx, id)
	if err != nil {
		return err
	}
	if mediaFinalPhase(task.Phase) {
		return nil
	}
	key, billingAccount := mediaBillingIdentity(task)
	save := func() error { return store.SaveTask(ctx, task, h.tasks.TaskTTL()) }
	defer func() {
		if task.LastError != "" {
			logger.L().Warn("media.task_pending", zap.String("task_id", id), zap.String("phase", task.Phase), zap.String("reason", task.LastError))
		}
	}()
	if task.Phase == mediaPhasePrepared || task.Phase == mediaPhaseReserving {
		task.Phase = mediaPhaseReserving
		if err := save(); err != nil {
			return err
		}
		_, err := h.tasks.Billing().Reserve(ctx, task, key, nil, billingAccount)
		if err != nil {
			if infraerrors.Reason(err) == "MEDIA_INSUFFICIENT_BALANCE" {
				task.Phase = mediaPhaseRejected
				task.Status = MediaTaskStatusFailed
				task.Error = "Insufficient balance"
				task.Request = nil
			} else {
				task.LastError = infraerrors.Reason(err)
				task.NextAttemptAt = time.Now().Add(30 * time.Second).Unix()
			}
			if saveErr := save(); saveErr != nil {
				return saveErr
			}
			return err
		}
		task.Phase = mediaPhaseReserved
		task.LastError = ""
		task.NextAttemptAt = time.Now().Unix()
		if err := save(); err != nil {
			return err
		}
	}
	if reserveOnly {
		return nil
	}
	if task.Phase == mediaPhaseReserved || task.Phase == mediaPhaseSubmitting || task.Phase == mediaPhaseUnknown {
		// Never indefinitely replay an ambiguous create after an unknown upstream
		// idempotency retention window. Hold remains durable for reconciliation.
		if task.Attempts >= 3 || (task.Attempts > 0 && time.Now().Unix()-task.CreatedAt > 600) {
			task.Phase = mediaPhaseReview
			task.LastError = "submission_outcome_unknown"
			task.Error = "Generation is awaiting confirmation"
			return save()
		}
		account, err := h.boundAccount(ctx, task)
		if err != nil {
			return h.deferTask(ctx, task, err)
		}
		release, err := h.acquireAccountSlot(ctx, account)
		if err != nil {
			return h.deferTask(ctx, task, err)
		}
		defer release()
		if task.Request == nil {
			return ErrMediaTaskUnavailable
		}
		task.Phase = mediaPhaseSubmitting
		task.Attempts++
		task.NextAttemptAt = time.Now().Add(30 * time.Second).Unix()
		if err := save(); err != nil {
			return err
		}
		job, err := h.tasks.Provider().Create(ctx, account, task.Request, task.UpstreamImageIDs, task.UpstreamKey)
		if err != nil {
			var rejected *SubmissionRejected
			if errors.As(err, &rejected) && task.Attempts == 1 {
				task.Phase = mediaPhaseReleasing
				task.Status = MediaTaskStatusFailed
				task.Error = "Video generation failed"
				task.LastError = infraerrors.Reason(err)
				if err := save(); err != nil {
					return err
				}
			} else {
				task.Phase = mediaPhaseUnknown
				return h.deferTask(ctx, task, err)
			}
		} else {
			if job == nil || strings.TrimSpace(job.JobID) == "" {
				task.Phase = mediaPhaseUnknown
				return h.deferTask(ctx, task, ErrMediaTaskUnavailable)
			}
			task.JobID = job.JobID
			task.UpstreamIdempotent = job.Idempotent
			task.Phase = mediaPhaseSubmitted
			task.LastError = ""
			task.Error = ""
			if status := NormalizeMediaStatus(job.Status); status != "" {
				task.Status = status
			}
			return save()
		}
	}
	if task.Phase == mediaPhaseSubmitted {
		account, err := h.boundAccount(ctx, task)
		if err != nil {
			return h.deferTask(ctx, task, err)
		}
		job, err := h.tasks.Provider().Status(ctx, account, task.JobID)
		if err != nil {
			return h.deferTask(ctx, task, err)
		}
		if job == nil || job.JobID != task.JobID || NormalizeMediaStatus(job.Status) == "" {
			return h.deferTask(ctx, task, infraerrors.New(502, "MEDIA_INVALID_STATUS", "unsupported upstream state"))
		}
		task.Status = NormalizeMediaStatus(job.Status)
		task.LastError = ""
		task.NextAttemptAt = time.Now().Add(15 * time.Second).Unix()
		if task.Status == MediaTaskStatusFailed {
			task.Phase = mediaPhaseReleasing
			task.Error = "Video generation failed"
			task.LastError = "upstream_task_failed"
		} else if task.Status == MediaTaskStatusSucceeded {
			ref, err := h.tasks.Provider().FetchDownloadRef(ctx, account, task.JobID)
			if err != nil {
				return h.deferTask(ctx, task, err)
			}
			if ref == nil || ref.NotReady {
				return save()
			}
			task.Phase = mediaPhaseCapturing
		}
		if IsTerminalMediaStatus(task.Status) && task.CompletedAt == nil {
			now := time.Now().Unix()
			task.CompletedAt = &now
		}
		if err := save(); err != nil {
			return err
		}
	}
	if task.Phase == mediaPhaseCapturing {
		if err := h.tasks.Billing().Capture(ctx, task, key, nil, billingAccount); err != nil {
			return h.deferTask(ctx, task, err)
		}
		task.Phase = mediaPhaseSettled
		task.Status = MediaTaskStatusSucceeded
		task.Error = ""
		task.LastError = ""
		task.Request = nil
		task.UpstreamImageIDs = nil
		return save()
	}
	if task.Phase == mediaPhaseReleasing {
		if err := h.tasks.Billing().Release(ctx, task, key); err != nil {
			return h.deferTask(ctx, task, err)
		}
		task.Phase = mediaPhaseReleased
		task.Status = MediaTaskStatusFailed
		task.Request = nil
		task.UpstreamImageIDs = nil
		return save()
	}
	return nil
}

func (h *MediaTaskHandler) deferTask(ctx context.Context, task *MediaTaskRecord, err error) error {
	// No raw upstream body, prompt, credentials or signed URL is logged/stored.
	task.LastError = infraerrors.Reason(err)
	if task.LastError == "" {
		task.LastError = "transport_or_storage_error"
	}
	task.NextAttemptAt = time.Now().Add(30 * time.Second).Unix()
	if task.Attempts > 0 && task.Phase == mediaPhaseUnknown {
		task.Error = "Generation is awaiting confirmation"
	}
	if saveErr := h.tasks.Store().SaveTask(ctx, task, h.tasks.TaskTTL()); saveErr != nil {
		return saveErr
	}
	return err
}
