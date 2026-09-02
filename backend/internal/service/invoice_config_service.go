package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/mail"
	"sort"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type InvoiceStorageReadiness interface {
	CheckReady(context.Context) (InvoiceStorageStatus, error)
}

type InvoiceConfigService struct {
	settings SettingRepository
	storage  InvoiceStorageReadiness
}

func NewInvoiceConfigService(settings SettingRepository, storage InvoiceStorageReadiness) *InvoiceConfigService {
	return &InvoiceConfigService{settings: settings, storage: storage}
}

func DefaultInvoiceConfig() InvoiceConfig {
	return InvoiceConfig{
		Enabled:                 false,
		MinAmount:               0.01,
		MaxAmount:               0,
		ApplicationDays:         0,
		MaxOrdersPerRequest:     50,
		ItemName:                "信息技术服务费",
		AdminNotificationEmails: []string{},
		MaxFileSizeMB:           10,
		AllowReapplyAfterVoid:   false,
	}
}

func (s *InvoiceConfigService) Get(ctx context.Context) (InvoiceConfig, error) {
	cfg := DefaultInvoiceConfig()
	if s == nil || s.settings == nil {
		return cfg, nil
	}
	setting, err := s.settings.Get(ctx, SettingKeyInvoiceConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return cfg, nil
		}
		return InvoiceConfig{}, err
	}
	if setting == nil || strings.TrimSpace(setting.Value) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(setting.Value), &cfg); err != nil {
		return InvoiceConfig{}, infraerrors.InternalServer("INVOICE_CONFIG_INVALID", "invoice configuration is invalid").WithCause(err)
	}
	cfg = normalizeInvoiceConfig(cfg)
	if err := validateInvoiceConfig(cfg); err != nil {
		return InvoiceConfig{}, err
	}
	gate, err := s.rolloutEnabled(ctx)
	if err != nil {
		return InvoiceConfig{}, err
	}
	// Keep the legacy JSON's enabled bit as a second safety condition while
	// requiring the independent namespaced rollout switch.  This preserves
	// old configuration for rollback without inheriting an old true value.
	cfg.Enabled = cfg.Enabled && gate
	return cfg, nil
}

func (s *InvoiceConfigService) rolloutEnabled(ctx context.Context) (bool, error) {
	if s == nil || s.settings == nil {
		return false, nil
	}
	setting, err := s.settings.Get(ctx, SettingKeySubNexusInvoiceEnabled)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return false, nil
		}
		return false, err
	}
	return setting != nil && setting.Value == "true", nil
}

func (s *InvoiceConfigService) Update(ctx context.Context, cfg InvoiceConfig, actor InvoiceAuditActor) (InvoiceConfig, error) {
	if s == nil || s.settings == nil {
		return InvoiceConfig{}, infraerrors.InternalServer("INVOICE_CONFIG_INVALID", "invoice configuration service is unavailable")
	}
	if actor.Type != "admin" || actor.ID == nil || *actor.ID <= 0 {
		return InvoiceConfig{}, infraerrors.Forbidden("INVOICE_ADMIN_REQUIRED", "administrator identity is required")
	}
	previous, err := s.Get(ctx)
	if err != nil {
		return InvoiceConfig{}, err
	}
	cfg = normalizeInvoiceConfig(cfg)
	if err := validateInvoiceConfig(cfg); err != nil {
		return InvoiceConfig{}, err
	}
	if cfg.Enabled {
		if s.storage == nil {
			return InvoiceConfig{}, infraerrors.Conflict("INVOICE_STORAGE_UNAVAILABLE", "invoice storage is unavailable")
		}
		if _, err := s.storage.CheckReady(ctx); err != nil {
			return InvoiceConfig{}, infraerrors.Conflict("INVOICE_STORAGE_UNAVAILABLE", "invoice storage is unavailable").WithCause(err)
		}
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return InvoiceConfig{}, err
	}
	audit, auditKey, err := newInvoiceConfigAudit(previous, cfg, actor)
	if err != nil {
		return InvoiceConfig{}, err
	}
	auditRaw, err := json.Marshal(audit)
	if err != nil {
		return InvoiceConfig{}, err
	}
	if err := s.settings.SetMultiple(ctx, map[string]string{
		SettingKeyInvoiceConfig:          string(raw),
		SettingKeySubNexusInvoiceEnabled: strconv.FormatBool(cfg.Enabled),
		auditKey:                         string(auditRaw),
	}); err != nil {
		return InvoiceConfig{}, err
	}
	return cfg, nil
}

func (s *InvoiceConfigService) ListAudits(ctx context.Context, limit int) ([]InvoiceConfigAuditEntry, error) {
	if s == nil || s.settings == nil {
		return nil, infraerrors.InternalServer("INVOICE_CONFIG_INVALID", "invoice configuration service is unavailable")
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	all, err := s.settings.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]InvoiceConfigAuditEntry, 0)
	for key, raw := range all {
		if !strings.HasPrefix(key, SettingKeyInvoiceConfigAudit) {
			continue
		}
		var item InvoiceConfigAuditEntry
		if json.Unmarshal([]byte(raw), &item) == nil && item.ID != "" {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func newInvoiceConfigAudit(previous, current InvoiceConfig, actor InvoiceAuditActor) (InvoiceConfigAuditEntry, string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return InvoiceConfigAuditEntry{}, "", err
	}
	now := time.Now().UTC()
	id := now.Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(random)
	entry := InvoiceConfigAuditEntry{
		ID:             id,
		AdminID:        *actor.ID,
		ChangedFields:  invoiceConfigChangedFields(previous, current),
		PreviousEnable: previous.Enabled,
		Enabled:        current.Enabled,
		IPAddress:      actor.IPAddress,
		UserAgentHash:  actor.UserAgentHash,
		CreatedAt:      now,
	}
	return entry, SettingKeyInvoiceConfigAudit + id, nil
}

func invoiceConfigChangedFields(previous, current InvoiceConfig) []string {
	fields := make([]string, 0, 9)
	if previous.Enabled != current.Enabled {
		fields = append(fields, "enabled")
	}
	if previous.MinAmount != current.MinAmount {
		fields = append(fields, "min_amount")
	}
	if previous.MaxAmount != current.MaxAmount {
		fields = append(fields, "max_amount")
	}
	if previous.ApplicationDays != current.ApplicationDays {
		fields = append(fields, "application_days")
	}
	if previous.MaxOrdersPerRequest != current.MaxOrdersPerRequest {
		fields = append(fields, "max_orders_per_request")
	}
	if previous.ItemName != current.ItemName {
		fields = append(fields, "item_name")
	}
	if !stringSlicesEqual(previous.AdminNotificationEmails, current.AdminNotificationEmails) {
		fields = append(fields, "admin_notification_emails")
	}
	if previous.MaxFileSizeMB != current.MaxFileSizeMB {
		fields = append(fields, "max_file_size_mb")
	}
	if previous.AllowReapplyAfterVoid != current.AllowReapplyAfterVoid {
		fields = append(fields, "allow_reapply_after_void")
	}
	return fields
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func normalizeInvoiceConfig(cfg InvoiceConfig) InvoiceConfig {
	cfg.MinAmount = math.Round(cfg.MinAmount*100) / 100
	cfg.MaxAmount = math.Round(cfg.MaxAmount*100) / 100
	cfg.ItemName = strings.TrimSpace(cfg.ItemName)
	seen := make(map[string]struct{}, len(cfg.AdminNotificationEmails))
	emails := make([]string, 0, len(cfg.AdminNotificationEmails))
	for _, value := range cfg.AdminNotificationEmails {
		email := strings.ToLower(strings.TrimSpace(value))
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		emails = append(emails, email)
	}
	sort.Strings(emails)
	cfg.AdminNotificationEmails = emails
	return cfg
}

func validateInvoiceConfig(cfg InvoiceConfig) error {
	invalid := func(message string) error {
		return infraerrors.BadRequest("INVOICE_CONFIG_INVALID", message)
	}
	if math.IsNaN(cfg.MinAmount) || math.IsInf(cfg.MinAmount, 0) || cfg.MinAmount <= 0 {
		return invalid("invoice minimum amount must be greater than zero")
	}
	if math.IsNaN(cfg.MaxAmount) || math.IsInf(cfg.MaxAmount, 0) || cfg.MaxAmount < 0 || (cfg.MaxAmount > 0 && cfg.MaxAmount < cfg.MinAmount) {
		return invalid("invoice maximum amount must be zero or no less than the minimum")
	}
	if cfg.ApplicationDays < 0 || cfg.ApplicationDays > 3650 {
		return invalid("invoice application days must be between 0 and 3650")
	}
	if cfg.MaxOrdersPerRequest < 1 || cfg.MaxOrdersPerRequest > 100 {
		return invalid("invoice order limit must be between 1 and 100")
	}
	if cfg.ItemName == "" || len([]rune(cfg.ItemName)) > 100 {
		return invalid("invoice item name is invalid")
	}
	if len(cfg.AdminNotificationEmails) > 10 {
		return invalid("invoice administrator email limit exceeded")
	}
	for _, value := range cfg.AdminNotificationEmails {
		parsed, err := mail.ParseAddress(value)
		if err != nil || !strings.EqualFold(parsed.Address, value) || len(value) > 255 {
			return invalid("invoice administrator email is invalid")
		}
	}
	if cfg.MaxFileSizeMB < 1 || cfg.MaxFileSizeMB > 20 {
		return invalid("invoice file size limit must be between 1 and 20 MiB")
	}
	return nil
}
