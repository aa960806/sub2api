package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var invoiceTaxpayerIDPattern = regexp.MustCompile(`^[0-9A-Z]{15,20}$`)

type InvoiceService struct {
	repo   InvoiceRepository
	config *InvoiceConfigService
	files  *InvoiceFileStore
	emails InvoiceEmailNotifier
}

func NewInvoiceService(repo InvoiceRepository, config *InvoiceConfigService, files *InvoiceFileStore, emails InvoiceEmailNotifier) *InvoiceService {
	return &InvoiceService{repo: repo, config: config, files: files, emails: emails}
}

func (s *InvoiceService) GetPublicConfig(ctx context.Context, userID int64) (InvoicePublicConfig, error) {
	cfg, err := s.getConfig(ctx)
	if err != nil {
		return InvoicePublicConfig{}, err
	}
	history, err := s.repo.HasHistory(ctx, userID)
	if err != nil {
		return InvoicePublicConfig{}, err
	}
	// Administrator recipient addresses and storage limits are not public API data.
	cfg.AdminNotificationEmails = nil
	cfg.MaxFileSizeMB = 0
	return InvoicePublicConfig{
		InvoiceConfig:     cfg,
		HasHistory:        history,
		AllowedOrderTypes: []string{payment.OrderTypeBalance, payment.OrderTypeSubscription, payment.OrderTypeFirstRechargeGift},
	}, nil
}

func (s *InvoiceService) ListEligibleOrders(ctx context.Context, userID int64, page, pageSize int, keyword string) (InvoiceEligibleOrdersResult, error) {
	cfg, err := s.requireEnabled(ctx)
	if err != nil {
		return InvoiceEligibleOrdersResult{}, err
	}
	return s.repo.ListEligibleOrders(ctx, userID, cfg, page, pageSize, keyword)
}

func (s *InvoiceService) Create(ctx context.Context, userID int64, input InvoiceCreateInput, actor InvoiceAuditActor) (*InvoiceRequest, error) {
	cfg, err := s.requireEnabled(ctx)
	if err != nil {
		return nil, err
	}
	header, err := normalizeAndValidateInvoiceHeader(input.InvoiceHeaderInput)
	if err != nil {
		return nil, err
	}
	request, err := s.repo.Create(ctx, InvoiceCreateParams{UserID: userID, OrderIDs: input.OrderIDs, Header: header, Config: cfg, Actor: actor})
	if err != nil {
		return nil, err
	}
	s.notifyDetached(func(notifyCtx context.Context) error { return s.emails.ApplicationSubmitted(notifyCtx, request) })
	return sanitizeInvoiceRequestForUser(request), nil
}

func (s *InvoiceService) ListMy(ctx context.Context, userID int64, page, pageSize int) ([]InvoiceRequest, int64, error) {
	items, total, err := s.repo.ListUser(ctx, userID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	for i := range items {
		sanitizeInvoiceRequestValueForUser(&items[i])
	}
	return items, total, nil
}

func (s *InvoiceService) GetMy(ctx context.Context, requestID, userID int64) (*InvoiceRequest, error) {
	request, err := s.repo.GetUser(ctx, requestID, userID)
	if err != nil {
		return nil, err
	}
	return sanitizeInvoiceRequestForUser(request), nil
}

func (s *InvoiceService) Cancel(ctx context.Context, requestID, userID int64, actor InvoiceAuditActor) (*InvoiceRequest, error) {
	// Cancellation releases an invoice order reservation and therefore remains
	// a feature write.  Keep it behind the same fail-closed switch as submit
	// and resubmit, even when the request is already in a terminal state.
	if _, err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	request, err := s.repo.Cancel(ctx, requestID, userID, actor)
	if err != nil {
		return nil, err
	}
	return sanitizeInvoiceRequestForUser(request), nil
}

func (s *InvoiceService) Resubmit(ctx context.Context, requestID, userID int64, input InvoiceResubmitInput, actor InvoiceAuditActor) (*InvoiceRequest, error) {
	cfg, err := s.requireEnabled(ctx)
	if err != nil {
		return nil, err
	}
	header, err := normalizeAndValidateInvoiceHeader(input.InvoiceHeaderInput)
	if err != nil {
		return nil, err
	}
	request, err := s.repo.Resubmit(ctx, InvoiceResubmitParams{RequestID: requestID, UserID: userID, Header: header, Config: cfg, Actor: actor})
	if err != nil {
		return nil, err
	}
	s.notifyDetached(func(notifyCtx context.Context) error { return s.emails.ApplicationSubmitted(notifyCtx, request) })
	return sanitizeInvoiceRequestForUser(request), nil
}

func (s *InvoiceService) GetAdminConfig(ctx context.Context) (InvoiceAdminConfigResult, error) {
	cfg, err := s.getConfig(ctx)
	if err != nil {
		return InvoiceAdminConfigResult{}, err
	}
	status := InvoiceStorageStatus{CheckedAt: time.Now()}
	if s.files != nil {
		status, _ = s.files.CheckReady(ctx)
	}
	audits, err := s.config.ListAudits(ctx, 20)
	if err != nil {
		return InvoiceAdminConfigResult{}, err
	}
	return InvoiceAdminConfigResult{Config: cfg, Storage: status, ConfigAudits: audits}, nil
}

func (s *InvoiceService) UpdateAdminConfig(ctx context.Context, cfg InvoiceConfig, actor InvoiceAuditActor) (InvoiceAdminConfigResult, error) {
	updated, err := s.config.Update(ctx, cfg, actor)
	if err != nil {
		return InvoiceAdminConfigResult{}, err
	}
	status := InvoiceStorageStatus{CheckedAt: time.Now()}
	if s.files != nil {
		status, _ = s.files.CheckReady(ctx)
	}
	audits, err := s.config.ListAudits(ctx, 20)
	if err != nil {
		return InvoiceAdminConfigResult{}, err
	}
	return InvoiceAdminConfigResult{Config: updated, Storage: status, ConfigAudits: audits}, nil
}

func (s *InvoiceService) ReconcileFiles(ctx context.Context) (InvoiceReconciliationReport, error) {
	report := InvoiceReconciliationReport{CheckedAt: time.Now().UTC(), MissingFiles: []InvoiceReconciliationEntry{}, OrphanStorageKeys: []string{}}
	if s == nil || s.repo == nil || s.files == nil {
		return report, infraerrors.InternalServer("INVOICE_CONFIG_INVALID", "invoice service is unavailable")
	}
	databaseFiles, err := s.repo.ListAllFiles(ctx)
	if err != nil {
		return report, err
	}
	storageKeys, err := s.files.ListStorageKeys(ctx)
	if err != nil {
		return report, err
	}
	report.DatabaseFileCount = len(databaseFiles)
	report.StorageFileCount = len(storageKeys)
	databaseKeys := make(map[string]struct{}, len(databaseFiles))
	storageSet := make(map[string]struct{}, len(storageKeys))
	for _, key := range storageKeys {
		storageSet[key] = struct{}{}
	}
	for _, file := range databaseFiles {
		databaseKeys[file.StorageKey] = struct{}{}
		if _, ok := storageSet[file.StorageKey]; !ok {
			report.MissingFiles = append(report.MissingFiles, InvoiceReconciliationEntry{FileID: file.ID, InvoiceRequestID: file.InvoiceRequestID, StorageKey: file.StorageKey, SHA256: file.SHA256})
		}
	}
	for _, key := range storageKeys {
		if _, ok := databaseKeys[key]; !ok {
			report.OrphanStorageKeys = append(report.OrphanStorageKeys, key)
		}
	}
	return report, nil
}

func (s *InvoiceService) ListAdmin(ctx context.Context, params InvoiceListParams) ([]InvoiceRequest, int64, error) {
	items, total, err := s.repo.ListAdmin(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	for i := range items {
		items[i].TaxpayerID = maskInvoiceIdentifier(items[i].TaxpayerID)
		items[i].BankAccount = maskInvoiceIdentifier(items[i].BankAccount)
		items[i].ConfigSnapshot = InvoiceConfig{}
	}
	return items, total, nil
}

func (s *InvoiceService) GetAdmin(ctx context.Context, requestID int64) (*InvoiceRequest, error) {
	return s.repo.GetAdmin(ctx, requestID)
}

func (s *InvoiceService) Accept(ctx context.Context, params InvoiceAdminActionParams) (*InvoiceRequest, error) {
	if _, err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	return s.repo.Accept(ctx, params)
}

func (s *InvoiceService) Release(ctx context.Context, params InvoiceAdminActionParams) (*InvoiceRequest, error) {
	if _, err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	params.Reason = strings.TrimSpace(params.Reason)
	return s.repo.Release(ctx, params)
}

func (s *InvoiceService) Reject(ctx context.Context, params InvoiceAdminActionParams) (*InvoiceRequest, error) {
	if _, err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	request, err := s.repo.Reject(ctx, params)
	if err != nil {
		return nil, err
	}
	s.notifyDetached(func(notifyCtx context.Context) error { return s.emails.ApplicationRejected(notifyCtx, request) })
	return request, nil
}

func (s *InvoiceService) Issue(ctx context.Context, params InvoiceIssueParams, upload InvoiceUploadInput) (*InvoiceRequest, error) {
	if _, err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	request, err := s.repo.GetAdmin(ctx, params.RequestID)
	if err != nil {
		return nil, err
	}
	if request.Status != InvoiceStatusProcessing && request.Status != InvoiceStatusIssued {
		return nil, infraerrors.Conflict("INVALID_INVOICE_STATUS_TRANSITION", "invoice request status changed")
	}
	params.InvoiceCode = strings.TrimSpace(params.InvoiceCode)
	params.InvoiceNumber = strings.TrimSpace(params.InvoiceNumber)
	params.InvoiceDate, err = normalizeInvoiceDate(params.InvoiceDate)
	if err != nil || params.InvoiceNumber == "" || len([]rune(params.InvoiceNumber)) > 128 || len([]rune(params.InvoiceCode)) > 64 {
		return nil, infraerrors.BadRequest("INVALID_INVOICE_HEADER", "invoice issue fields are invalid")
	}
	if s.files == nil {
		return nil, infraerrors.Conflict("INVOICE_STORAGE_UNAVAILABLE", "invoice storage is unavailable")
	}
	prepared, err := s.files.Prepare(ctx, params.RequestID, params.AdminID, upload, int64(request.ConfigSnapshot.MaxFileSizeMB)<<20)
	if err != nil {
		return nil, err
	}
	params.File = prepared.Metadata
	result, err := s.repo.Issue(ctx, params)
	result, err = s.resolvePreparedFileResult(ctx, prepared, result, err)
	if err != nil {
		return nil, err
	}
	s.notifyDetached(func(notifyCtx context.Context) error { return s.emails.InvoiceIssued(notifyCtx, result) })
	return result, nil
}

func (s *InvoiceService) ReplaceFile(ctx context.Context, params InvoiceReplaceFileParams, upload InvoiceUploadInput) (*InvoiceRequest, error) {
	if _, err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	request, err := s.repo.GetAdmin(ctx, params.RequestID)
	if err != nil {
		return nil, err
	}
	if request.Status != InvoiceStatusIssued {
		return nil, infraerrors.Conflict("INVALID_INVOICE_STATUS_TRANSITION", "invoice request status changed")
	}
	params.InvoiceDate, err = normalizeInvoiceDate(params.InvoiceDate)
	if err != nil {
		return nil, err
	}
	if s.files == nil {
		return nil, infraerrors.Conflict("INVOICE_STORAGE_UNAVAILABLE", "invoice storage is unavailable")
	}
	prepared, err := s.files.Prepare(ctx, params.RequestID, params.AdminID, upload, int64(request.ConfigSnapshot.MaxFileSizeMB)<<20)
	if err != nil {
		return nil, err
	}
	params.File = prepared.Metadata
	result, err := s.repo.ReplaceFile(ctx, params)
	return s.resolvePreparedFileResult(ctx, prepared, result, err)
}

func (s *InvoiceService) resolvePreparedFileResult(ctx context.Context, prepared *InvoicePreparedFile, result *InvoiceRequest, persistenceErr error) (*InvoiceRequest, error) {
	if prepared == nil || s == nil || s.files == nil {
		if persistenceErr != nil {
			return nil, persistenceErr
		}
		return result, nil
	}
	if invoiceRequestReferencesPreparedFile(result, prepared) {
		return result, nil
	}
	if persistenceErr == nil {
		_ = s.files.DeletePrepared(prepared)
		if result == nil {
			return nil, infraerrors.InternalServer("INVOICE_FILE_PERSISTENCE_FAILED", "invoice file persistence result is missing")
		}
		return result, nil
	}

	// A transaction can commit successfully and still fail while loading the
	// response. Only remove the prepared file after a follow-up read proves it
	// is not referenced; an uncertain file is safer as a reconcilable orphan.
	current, lookupErr := s.repo.GetAdmin(ctx, prepared.Metadata.InvoiceRequestID)
	if lookupErr == nil {
		if invoiceRequestReferencesPreparedFile(current, prepared) {
			return current, nil
		}
		_ = s.files.DeletePrepared(prepared)
	}
	return nil, persistenceErr
}

func invoiceRequestReferencesPreparedFile(request *InvoiceRequest, prepared *InvoicePreparedFile) bool {
	return request != nil && request.CurrentFile != nil && prepared != nil &&
		request.CurrentFile.StorageKey == prepared.Metadata.StorageKey
}

func (s *InvoiceService) Void(ctx context.Context, params InvoiceAdminActionParams) (*InvoiceRequest, error) {
	if _, err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	return s.repo.Void(ctx, params)
}

func (s *InvoiceService) ListAuditLogs(ctx context.Context, requestID int64) ([]InvoiceAuditLog, error) {
	if _, err := s.repo.GetAdmin(ctx, requestID); err != nil {
		return nil, err
	}
	return s.repo.ListAuditLogs(ctx, requestID)
}

func (s *InvoiceService) DownloadMy(ctx context.Context, requestID, userID int64) (*InvoiceDownload, error) {
	_, file, err := s.repo.GetCurrentFileForUser(ctx, requestID, userID)
	if err != nil {
		return nil, err
	}
	return s.openDownload(file)
}

func (s *InvoiceService) DownloadAdmin(ctx context.Context, requestID int64) (*InvoiceDownload, error) {
	_, file, err := s.repo.GetCurrentFileForAdmin(ctx, requestID)
	if err != nil {
		return nil, err
	}
	return s.openDownload(file)
}

func (s *InvoiceService) ResendEmail(ctx context.Context, requestID int64) (*InvoiceRequest, error) {
	if _, err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	request, err := s.repo.GetAdmin(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if s.emails == nil {
		return nil, infraerrors.Conflict("INVOICE_EMAIL_UNAVAILABLE", "invoice email service is unavailable")
	}
	switch request.Status {
	case InvoiceStatusPending, InvoiceStatusProcessing:
		err = s.emails.ApplicationSubmitted(ctx, request)
	case InvoiceStatusRejected:
		err = s.emails.ApplicationRejected(ctx, request)
	case InvoiceStatusIssued:
		err = s.emails.InvoiceIssued(ctx, request)
	default:
		err = infraerrors.Conflict("INVALID_INVOICE_STATUS_TRANSITION", "invoice request has no resendable email")
	}
	return request, err
}

func (s *InvoiceService) openDownload(file *InvoiceFileMetadata) (*InvoiceDownload, error) {
	if s.files == nil || file == nil {
		return nil, infraerrors.NotFound("INVOICE_FILE_NOT_FOUND", "invoice file was not found")
	}
	reader, err := s.files.OpenVerified(*file)
	if err != nil {
		return nil, err
	}
	return &InvoiceDownload{Reader: reader, Metadata: *file}, nil
}

func (s *InvoiceService) getConfig(ctx context.Context) (InvoiceConfig, error) {
	if s == nil || s.config == nil || s.repo == nil {
		return InvoiceConfig{}, infraerrors.InternalServer("INVOICE_CONFIG_INVALID", "invoice service is unavailable")
	}
	return s.config.Get(ctx)
}

func (s *InvoiceService) requireEnabled(ctx context.Context) (InvoiceConfig, error) {
	cfg, err := s.getConfig(ctx)
	if err != nil {
		return InvoiceConfig{}, err
	}
	if !cfg.Enabled {
		return InvoiceConfig{}, infraerrors.Forbidden("INVOICE_DISABLED", "invoice applications are disabled")
	}
	return cfg, nil
}

func (s *InvoiceService) notifyDetached(send func(context.Context) error) {
	if s == nil || s.emails == nil || send == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := send(ctx); err != nil {
		slog.Warn("invoice notification email failed", "reason", infraerrors.Reason(err), "err", err.Error())
	}
}

func normalizeAndValidateInvoiceHeader(input InvoiceHeaderInput) (InvoiceHeaderInput, error) {
	input.TitleType = strings.ToUpper(strings.TrimSpace(input.TitleType))
	input.TitleName = strings.TrimSpace(input.TitleName)
	input.TaxpayerID = strings.ToUpper(strings.TrimSpace(input.TaxpayerID))
	input.RecipientEmail = strings.ToLower(strings.TrimSpace(input.RecipientEmail))
	input.RecipientPhone = strings.TrimSpace(input.RecipientPhone)
	input.CompanyAddress = strings.TrimSpace(input.CompanyAddress)
	input.CompanyPhone = strings.TrimSpace(input.CompanyPhone)
	input.BankName = strings.TrimSpace(input.BankName)
	input.BankAccount = strings.TrimSpace(input.BankAccount)
	input.UserNote = strings.TrimSpace(input.UserNote)
	for _, value := range []string{input.TitleName, input.TaxpayerID, input.RecipientEmail, input.RecipientPhone, input.CompanyAddress, input.CompanyPhone, input.BankName, input.BankAccount, input.UserNote} {
		if containsInvoiceControl(value) {
			return InvoiceHeaderInput{}, invalidInvoiceHeader()
		}
	}
	if input.TitleName == "" || len([]rune(input.TitleName)) > 200 || len([]rune(input.UserNote)) > 500 {
		return InvoiceHeaderInput{}, invalidInvoiceHeader()
	}
	parsed, err := mail.ParseAddress(input.RecipientEmail)
	if err != nil || !strings.EqualFold(parsed.Address, input.RecipientEmail) || len(input.RecipientEmail) > 255 {
		return InvoiceHeaderInput{}, invalidInvoiceHeader()
	}
	switch input.TitleType {
	case InvoiceTitlePersonal:
		if input.TaxpayerID != "" {
			return InvoiceHeaderInput{}, invalidInvoiceHeader()
		}
		input.CompanyAddress, input.CompanyPhone, input.BankName, input.BankAccount = "", "", "", ""
	case InvoiceTitleCompany:
		if !invoiceTaxpayerIDPattern.MatchString(input.TaxpayerID) {
			return InvoiceHeaderInput{}, invalidInvoiceHeader()
		}
	default:
		return InvoiceHeaderInput{}, invalidInvoiceHeader()
	}
	if len([]rune(input.RecipientPhone)) > 32 || len([]rune(input.CompanyAddress)) > 255 || len([]rune(input.CompanyPhone)) > 32 || len([]rune(input.BankName)) > 100 || len([]rune(input.BankAccount)) > 64 {
		return InvoiceHeaderInput{}, invalidInvoiceHeader()
	}
	return input, nil
}

func normalizeInvoiceDate(value time.Time) (time.Time, error) {
	if value.IsZero() {
		return time.Time{}, infraerrors.BadRequest("INVALID_INVOICE_HEADER", "invoice date is required")
	}
	date := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
	tomorrow := time.Now().UTC().Add(24 * time.Hour)
	if date.After(tomorrow) {
		return time.Time{}, infraerrors.BadRequest("INVALID_INVOICE_HEADER", "invoice date is invalid")
	}
	return date, nil
}

func NewInvoiceAuditActor(actorType string, actorID int64, ipAddress, userAgent string) InvoiceAuditActor {
	hash := sha256.Sum256([]byte(userAgent))
	var id *int64
	if actorID > 0 {
		value := actorID
		id = &value
	}
	return InvoiceAuditActor{Type: actorType, ID: id, IPAddress: strings.TrimSpace(ipAddress), UserAgentHash: hex.EncodeToString(hash[:])}
}

func containsInvoiceControl(value string) bool {
	for _, char := range value {
		if unicode.IsControl(char) {
			return true
		}
	}
	return false
}

func invalidInvoiceHeader() error {
	return infraerrors.BadRequest("INVALID_INVOICE_HEADER", "invoice header information is invalid")
}

func (s *InvoiceService) String() string {
	return fmt.Sprintf("InvoiceService(enabled_config=%t)", s != nil && s.config != nil)
}

func sanitizeInvoiceRequestForUser(request *InvoiceRequest) *InvoiceRequest {
	if request == nil {
		return nil
	}
	sanitizeInvoiceRequestValueForUser(request)
	return request
}

func sanitizeInvoiceRequestValueForUser(request *InvoiceRequest) {
	if request == nil {
		return
	}
	request.AdminNote = ""
	request.ConfigSnapshot = InvoiceConfig{}
	request.AcceptedBy = nil
	request.IssuedBy = nil
	request.RejectedBy = nil
	request.VoidedBy = nil
	if request.CurrentFile != nil {
		request.CurrentFile.UploadedBy = 0
	}
}

func maskInvoiceIdentifier(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= 4 {
		if len(runes) == 0 {
			return ""
		}
		return strings.Repeat("*", len(runes))
	}
	return strings.Repeat("*", len(runes)-4) + string(runes[len(runes)-4:])
}
