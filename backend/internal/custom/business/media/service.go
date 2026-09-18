package media

import (
	"context"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	"github.com/google/uuid"
)

// 媒体任务状态。与上游 Seedance 的四态一一对应（OpenAPI 的 VideoJob.status
// 枚举），未知状态一律 fail-closed，不做猜测映射。
const (
	MediaTaskStatusQueued    = "queued"
	MediaTaskStatusRunning   = "running"
	MediaTaskStatusSucceeded = "succeeded"
	MediaTaskStatusFailed    = "failed"
)

const (
	// Uploaded reference lifetime. Durable task intents do not expire until resolved.
	defaultMediaTaskTTL = 24 * time.Hour
	// Retained for the supplied store interface; durable settlement claims do not expire.
	defaultMediaSettlementTTL = 48 * time.Hour
)

var (
	// ErrMediaTaskNotFound 同时覆盖「不存在」「已过期」「不属于当前 API Key」三种情况。
	// 三者刻意不做区分，避免跨用户探测任务是否存在。
	ErrMediaTaskNotFound = infraerrors.New(http.StatusNotFound, "MEDIA_TASK_NOT_FOUND", "media task not found")
	// ErrMediaTaskUnavailable 用于存储不可用等 fail-closed 场景。
	ErrMediaTaskUnavailable = infraerrors.New(http.StatusServiceUnavailable, "MEDIA_TASK_UNAVAILABLE", "media task storage is unavailable")
	ErrMediaTaskDisabled    = infraerrors.New(http.StatusNotFound, "MEDIA_TASK_DISABLED", "media task API is not enabled")
	// ErrMediaFileCrossAccount 对应参考图跨账号引用：上游的 image_id 按签发它的
	// Key 隔离，混用不同账号的图片必然被上游 404。
	ErrMediaFileCrossAccount = infraerrors.New(http.StatusBadRequest, "MEDIA_FILE_CROSS_ACCOUNT", "reference images belong to different upstream accounts")
	// ErrMediaSubscriptionUnsupported 是订阅用户的 fail-closed 护栏：两阶段冻结只
	// 作用于余额，订阅额度没有冻结语义，静默扣余额等于对已付费用户双重收费。
	ErrMediaSubscriptionUnsupported = infraerrors.New(http.StatusForbidden, "MEDIA_SUBSCRIPTION_UNSUPPORTED", "media tasks are not available for subscription billing")
	ErrMediaAccountUnavailable      = infraerrors.New(http.StatusServiceUnavailable, "MEDIA_ACCOUNT_UNAVAILABLE", "no eligible upstream account")
)

// MediaTaskRecord is the private durable task record; owner and upstream IDs are never exposed.
type MediaTaskRecord struct {
	// Durable intent and billing snapshot. Never store API credentials here.
	RequestHash         string                   `json:"request_hash"`
	Request             *MediaVideoCreateRequest `json:"request,omitempty"`
	UpstreamImageIDs    []string                 `json:"upstream_image_ids,omitempty"`
	UpstreamKey         string                   `json:"upstream_key"`
	CredentialHash      string                   `json:"credential_hash"`
	Phase               string                   `json:"phase"`
	LastError           string                   `json:"last_error,omitempty"`
	NextAttemptAt       int64                    `json:"next_attempt_at"`
	Attempts            int                      `json:"attempts"`
	BillingQuota        bool                     `json:"billing_quota"`
	BillingRateLimits   bool                     `json:"billing_rate_limits"`
	BillingAccountQuota bool                     `json:"billing_account_quota"`
	BillingAccountType  string                   `json:"billing_account_type"`
	ID                  string                   `json:"id"`
	UserID              int64                    `json:"user_id"`
	APIKeyID            int64                    `json:"api_key_id"`
	// AccountID 是创建时选定的上游账号。上游的 job_id 按账号隔离，因此状态、
	// 下载与结算必须固定使用该账号，禁止跨账号 failover。
	AccountID int64  `json:"account_id"`
	GroupID   *int64 `json:"group_id,omitempty"`
	// JobID 是上游任务标识，只在内部使用，不作为对外路径参数。
	JobID           string   `json:"job_id"`
	Platform        string   `json:"platform"`
	Model           string   `json:"model"`
	DurationSeconds int      `json:"duration_seconds"`
	Resolution      string   `json:"resolution,omitempty"`
	Ratio           string   `json:"ratio,omitempty"`
	CameraMovement  string   `json:"camera_movement,omitempty"`
	FileIDs         []string `json:"file_ids,omitempty"`
	Status          string   `json:"status"`
	// UnitPrice 是创建时冻结所用的单价快照。结算必须用它而不是重新解析价格：
	// 上游 capture 要求实扣不超过冻结额，在途调价会让重新解析的价格卡死结算。
	UnitPrice float64 `json:"unit_price"`
	// UpstreamIdempotent 记录上游是否把本次创建标记为幂等重放，仅供对账。
	UpstreamIdempotent bool   `json:"upstream_idempotent,omitempty"`
	Error              string `json:"error,omitempty"`
	CreatedAt          int64  `json:"created_at"`
	CompletedAt        *int64 `json:"completed_at,omitempty"`
}

// MediaTask 是对外视图，刻意不含 UserID / APIKeyID / AccountID / JobID。
type MediaTask struct {
	TaskID          string `json:"task_id"`
	Object          string `json:"object"`
	Status          string `json:"status"`
	Model           string `json:"model"`
	DurationSeconds int    `json:"duration_seconds"`
	// Downloadable 为 false 且状态为 succeeded，表示上游结果尚未就绪
	// （取签名地址返回 409），客户端应继续轮询而非当作失败。
	Downloadable bool   `json:"downloadable"`
	Error        string `json:"error,omitempty"`
	CreatedAt    int64  `json:"created_at"`
	CompletedAt  *int64 `json:"completed_at,omitempty"`
}

// MediaFileRecord 是参考图的私有表示。AccountID 决定了引用它的任务只能落在同一账号。
type MediaFileRecord struct {
	ID        string `json:"id"`
	UserID    int64  `json:"user_id"`
	APIKeyID  int64  `json:"api_key_id"`
	AccountID int64  `json:"account_id"`
	Platform  string `json:"platform"`
	// ImageID 是上游签发的参考图标识，按账号隔离。
	ImageID   string `json:"image_id"`
	Format    string `json:"format"`
	Size      int64  `json:"size"`
	ExpiresAt int64  `json:"expires_at"`
	CreatedAt int64  `json:"created_at"`
}

// MediaFile 是参考图的对外视图。
type MediaFile struct {
	FileID    string `json:"file_id"`
	Object    string `json:"object"`
	Format    string `json:"format"`
	Size      int64  `json:"size"`
	ExpiresAt int64  `json:"expires_at"`
}

// MediaPendingHold preserves the supplied store contract. The production worker
// recovers the durable task journal; age alone never authorizes a refund.
type MediaPendingHold struct {
	TaskID   string  `json:"task_id"`
	UserID   int64   `json:"user_id"`
	APIKeyID int64   `json:"api_key_id"`
	Amount   float64 `json:"amount"`
	// DueAt is a reconciliation hint, never evidence that upstream work failed.
	DueAt int64 `json:"due_at"`
}

// MediaTaskStore 是媒体任务与参考图的窄存储接口。
//
// 刻意不扩展 GatewayCache：那里承载的是网关粘性会话与 Grok 视频专用前缀，
// 媒体任务有独立的生命周期与 TTL，混在一起会让两边的回滚互相牵连。
type MediaTaskStore interface {
	SaveTask(ctx context.Context, task *MediaTaskRecord, ttl time.Duration) error
	GetTask(ctx context.Context, id string) (*MediaTaskRecord, error)
	SaveFile(ctx context.Context, file *MediaFileRecord, ttl time.Duration) error
	GetFile(ctx context.Context, id string) (*MediaFileRecord, error)
	// ClaimSettlement 原子占位，返回 true 表示本次调用赢得结算权。
	ClaimSettlement(ctx context.Context, taskID string, ttl time.Duration) (bool, error)
	// ReleaseSettlement 释放占位，供结算失败后重试。
	ReleaseSettlement(ctx context.Context, taskID string) error

	// TrackPendingHold 保留原包待结清凭据接口；到期只用于核对，不自动退款。
	TrackPendingHold(ctx context.Context, hold *MediaPendingHold, ttl time.Duration) error
	// ListDuePendingHolds 返回已到期（DueAt <= now）且仍未结清的冻结额。
	ListDuePendingHolds(ctx context.Context, now time.Time, limit int) ([]*MediaPendingHold, error)
	// DeletePendingHold 在确认扣除或退回后清除索引。
	DeletePendingHold(ctx context.Context, taskID string) error
}

// MediaCapabilities 是 provider 对外声明的能力表，供 GET /v1/media/models 渲染。
type MediaCapabilities struct {
	Platform    string                 `json:"platform"`
	Models      []MediaModelCapability `json:"models"`
	Ratios      []string               `json:"ratios"`
	Resolutions []string               `json:"resolutions"`
	CameraMoves []string               `json:"camera_movement"`
	// MaxImageBytes 是单张参考图解码后的上限。
	MaxImageBytes int64 `json:"max_image_bytes"`
	// MaxImagesTotalBytes 是多图合计上限。
	MaxImagesTotalBytes int64 `json:"max_images_total_bytes"`
}

// MediaModelCapability 描述单个模型的合法时长与参考图张数上限。
type MediaModelCapability struct {
	ID           string `json:"id"`
	Durations    []int  `json:"durations"`
	MaxRefImages int    `json:"max_reference_images"`
}

// MediaVideoCreateRequest 是创建视频任务的归一化入参。
type MediaVideoCreateRequest struct {
	Prompt          string
	Model           string
	DurationSeconds int
	Ratio           string
	Resolution      string
	CameraMovement  string
	// FileIDs 是 SUB2 侧参考图 id，由 service 层解析为上游 image_id。
	FileIDs []string
	// ImageURLs 是用户直接提供的公网直链，由上游自行拉取，不产生账号亲和。
	ImageURLs []string
}

// MediaProviderJob 是 provider 返回的上游任务快照。
type MediaProviderJob struct {
	JobID  string
	Status string
	Error  string
	// Idempotent 为真表示上游判定本次创建是同一幂等键的重放。
	Idempotent bool
}

// MediaDownloadRef 是一次性下载引用，不持久化。
type MediaDownloadRef struct {
	// URL 已由 provider 完成安全校验并补全为可直接请求的绝对地址。
	URL       string
	ExpiresAt int64
	// NotReady 为真表示上游结果尚未就绪（上游对未就绪返回 409 而非 404），
	// 调用方应返回「成功但暂不可下载」而不是失败。
	NotReady bool
}

// MediaDownloadResponse 是透传给客户端的下载响应。
type MediaDownloadResponse struct {
	StatusCode int
	Header     http.Header
	Body       interface{ Read([]byte) (int, error) }
	Close      func() error
}

// MediaProviderFile 是 provider 上传参考图后的结果。
type MediaProviderFile struct {
	ImageID   string
	Format    string
	Size      int64
	ExpiresAt int64
}

// MediaProvider 是媒体任务的上游适配接口。
//
// 该接口在 S02 冻结，S03（Seedance）只实现不修改。新增 provider 时也必须
// 落在本接口内，不得在 handler 或 service 层为某个 provider 开特例分支。
type MediaProvider interface {
	Platform() string
	Capabilities() MediaCapabilities
	// ValidateCreate 做 provider 侧的入参合法性校验（模型、时长、张数上限等），
	// 在冻结余额与调用上游之前执行。
	ValidateCreate(req *MediaVideoCreateRequest) error
	Create(ctx context.Context, account *Account, req *MediaVideoCreateRequest, upstreamImageIDs []string, idempotencyKey string) (*MediaProviderJob, error)
	Status(ctx context.Context, account *Account, jobID string) (*MediaProviderJob, error)
	FetchDownloadRef(ctx context.Context, account *Account, jobID string) (*MediaDownloadRef, error)
	Download(ctx context.Context, account *Account, ref *MediaDownloadRef, rangeHeader string) (*MediaDownloadResponse, error)
	UploadReferenceImage(ctx context.Context, account *Account, imageB64 string) (*MediaProviderFile, error)
	ProbeCredential(ctx context.Context, account *Account) error
}

// MediaBilling reuses the supplied two-stage ledger. Missing billing disables admission.
//
// 拆成接口而不是直接调用，是为了让 S02 的骨架能用假 provider 独立验收，
// 同时让 S04 的接入是纯新增而不需要重排本文件的调用顺序。
type MediaBilling interface {
	// Reserve 在向上游建单之前冻结余额，返回冻结所用的单价快照。
	Reserve(ctx context.Context, task *MediaTaskRecord, apiKey *APIKey, user *User, account *Account) (float64, error)
	// Capture 在首次观察到终态成功且结果可下载时确认扣除。
	Capture(ctx context.Context, task *MediaTaskRecord, apiKey *APIKey, user *User, account *Account) error
	// Release 在创建失败、任务失败或过期时退回冻结额。
	Release(ctx context.Context, task *MediaTaskRecord, apiKey *APIKey) error
}

// MediaTaskService 承载媒体任务的生命周期，不含任何 provider 协议细节。
type MediaTaskService struct {
	acceptNew bool
	store     MediaTaskStore
	// platform 独立于 provider 保存：provider 未接入（S02 阶段为 nil）时，
	// 平台门禁与在途任务轮询仍需正常工作。
	platform string
	provider MediaProvider
	billing  MediaBilling
	taskTTL  time.Duration
	claimTTL time.Duration
}

func NewMediaTaskService(store MediaTaskStore, platform string, provider MediaProvider) *MediaTaskService {
	return &MediaTaskService{
		acceptNew: true,
		store:     store,
		platform:  platform,
		provider:  provider,
		taskTTL:   defaultMediaTaskTTL,
		claimTTL:  defaultMediaSettlementTTL,
	}
}

// WithBilling 注入结算实现（S04）。返回自身以便链式构造。
func (s *MediaTaskService) WithBilling(b MediaBilling) *MediaTaskService {
	if s != nil {
		s.billing = b
	}
	return s
}

// Enabled 报告是否接受新建任务与上传。store 或 provider 缺失时整体禁用。
func (s *MediaTaskService) Enabled() bool {
	return s != nil && s.acceptNew && s.store != nil && s.provider != nil && s.billing != nil
}

func (s *MediaTaskService) WithEnabled(enabled bool) *MediaTaskService {
	if s != nil {
		s.acceptNew = enabled
	}
	return s
}

// Pollable 弱于 Enabled：总开关关闭后，在途任务仍可查询与下载，
// 避免已冻结余额的任务被搁死。
func (s *MediaTaskService) Pollable() bool {
	return s != nil && s.store != nil
}

func (s *MediaTaskService) Provider() MediaProvider { return s.provider }

// Platform 返回本接口服务的平台，不依赖 provider 是否已接入。
func (s *MediaTaskService) Platform() string {
	if s == nil {
		return ""
	}
	return s.platform
}

func (s *MediaTaskService) Store() MediaTaskStore { return s.store }

func (s *MediaTaskService) Billing() MediaBilling { return s.billing }

func (s *MediaTaskService) TaskTTL() time.Duration { return s.taskTTL }

func (s *MediaTaskService) ClaimTTL() time.Duration { return s.claimTTL }

// TrackHold retains compatibility with the source package. Production recovery
// uses the journal persisted before Reserve, not an expiring secondary index.
func (s *MediaTaskService) TrackHold(ctx context.Context, task *MediaTaskRecord) error {
	if s == nil || s.store == nil || task == nil || task.UnitPrice <= 0 {
		return nil
	}
	return s.store.TrackPendingHold(ctx, &MediaPendingHold{
		TaskID:   task.ID,
		UserID:   task.UserID,
		APIKeyID: task.APIKeyID,
		Amount:   task.UnitPrice,
		DueAt:    time.Now().Add(s.taskTTL).Unix(),
	}, s.claimTTL)
}

// UntrackHold 在冻结额已结清（确认扣除或已退回）后清除索引。
func (s *MediaTaskService) UntrackHold(ctx context.Context, taskID string) {
	if s == nil || s.store == nil {
		return
	}
	_ = s.store.DeletePendingHold(ctx, taskID)
}

// NewMediaID 生成对外 ID。刻意与上游标识解耦：对外契约不应绑定某个 provider 的 ID 形态。
func NewMediaID() string { return uuid.NewString() }

// ResolveTask 读取任务并校验归属。不存在、已过期与不属于当前 (user, apiKey)
// 一律返回 ErrMediaTaskNotFound，读取失败返回 ErrMediaTaskUnavailable。
func (s *MediaTaskService) ResolveTask(ctx context.Context, taskID string, userID, apiKeyID int64) (*MediaTaskRecord, error) {
	if !s.Pollable() {
		return nil, ErrMediaTaskUnavailable
	}
	if strings.TrimSpace(taskID) == "" {
		return nil, ErrMediaTaskNotFound
	}
	rec, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		if infraerrors.Code(err) == infraerrors.Code(ErrMediaTaskNotFound) {
			return nil, ErrMediaTaskNotFound
		}
		return nil, ErrMediaTaskUnavailable.WithCause(err)
	}
	if rec == nil || rec.UserID != userID || rec.APIKeyID != apiKeyID {
		return nil, ErrMediaTaskNotFound
	}
	return rec, nil
}

// ResolveFiles 把 SUB2 侧 file id 解析为上游 image_id，并强制账号亲和。
//
// 上游的 image_id 按签发它的 Key 隔离，因此一次创建引用的全部参考图必须来自
// 同一账号；否则上游必然 404。返回的 accountID 用于锁定创建时的账号选择。
func (s *MediaTaskService) ResolveFiles(ctx context.Context, fileIDs []string, userID, apiKeyID int64) (upstreamIDs []string, accountID int64, err error) {
	if len(fileIDs) == 0 {
		return nil, 0, nil
	}
	if !s.Pollable() {
		return nil, 0, ErrMediaTaskUnavailable
	}
	now := time.Now().Unix()
	for _, fid := range fileIDs {
		rec, getErr := s.store.GetFile(ctx, fid)
		if getErr != nil {
			if infraerrors.Code(getErr) == infraerrors.Code(ErrMediaTaskNotFound) {
				return nil, 0, ErrMediaTaskNotFound
			}
			return nil, 0, ErrMediaTaskUnavailable.WithCause(getErr)
		}
		if rec == nil || rec.UserID != userID || rec.APIKeyID != apiKeyID {
			return nil, 0, ErrMediaTaskNotFound
		}
		// 上游参考图会过期，过期后引用它必然失败，提前拦掉。
		if rec.ExpiresAt > 0 && rec.ExpiresAt <= now {
			return nil, 0, ErrMediaTaskNotFound
		}
		if accountID == 0 {
			accountID = rec.AccountID
		} else if rec.AccountID != accountID {
			return nil, 0, ErrMediaFileCrossAccount
		}
		upstreamIDs = append(upstreamIDs, rec.ImageID)
	}
	return upstreamIDs, accountID, nil
}

// PublicTask 把私有记录转换为对外视图。downloadable 由调用方按是否取到有效
// 下载引用给出，不从记录推导。
func (r *MediaTaskRecord) PublicTask(downloadable bool) *MediaTask {
	if r == nil {
		return nil
	}
	return &MediaTask{
		TaskID:          r.ID,
		Object:          "media.video_task",
		Status:          r.Status,
		Model:           r.Model,
		DurationSeconds: r.DurationSeconds,
		Downloadable:    downloadable,
		Error:           r.Error,
		CreatedAt:       r.CreatedAt,
		CompletedAt:     r.CompletedAt,
	}
}

// PublicFile 把私有参考图记录转换为对外视图。
func (r *MediaFileRecord) PublicFile() *MediaFile {
	if r == nil {
		return nil
	}
	return &MediaFile{
		FileID:    r.ID,
		Object:    "media.file",
		Format:    r.Format,
		Size:      r.Size,
		ExpiresAt: r.ExpiresAt,
	}
}

// IsTerminalMediaStatus 报告状态是否为终态。
func IsTerminalMediaStatus(status string) bool {
	switch status {
	case MediaTaskStatusSucceeded, MediaTaskStatusFailed:
		return true
	default:
		return false
	}
}

// NormalizeMediaStatus 校验上游状态是否为已知四态。未知状态返回空串，
// 调用方必须据此 fail-closed，不得猜测映射。
func NormalizeMediaStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case MediaTaskStatusQueued:
		return MediaTaskStatusQueued
	case MediaTaskStatusRunning:
		return MediaTaskStatusRunning
	case MediaTaskStatusSucceeded:
		return MediaTaskStatusSucceeded
	case MediaTaskStatusFailed:
		return MediaTaskStatusFailed
	default:
		return ""
	}
}
