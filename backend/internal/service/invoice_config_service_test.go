package service

import (
	"context"
	"errors"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type invoiceConfigSettingRepo struct {
	values map[string]string
}

func (r *invoiceConfigSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}
func (r *invoiceConfigSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	value, err := r.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return value.Value, nil
}
func (r *invoiceConfigSettingRepo) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}
func (r *invoiceConfigSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("not implemented")
}
func (r *invoiceConfigSettingRepo) SetMultiple(context.Context, map[string]string) error {
	return errors.New("use SetMultiple implementation")
}
func (r *invoiceConfigSettingRepo) GetAll(context.Context) (map[string]string, error) {
	values := make(map[string]string, len(r.values))
	for key, value := range r.values {
		values[key] = value
	}
	return values, nil
}
func (r *invoiceConfigSettingRepo) Delete(context.Context, string) error { return nil }

type invoiceConfigAtomicSettingRepo struct {
	invoiceConfigSettingRepo
	setMultipleCalls int
}

func (r *invoiceConfigAtomicSettingRepo) SetMultiple(_ context.Context, values map[string]string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	for key, value := range values {
		r.values[key] = value
	}
	r.setMultipleCalls++
	return nil
}

func invoiceAdminActor() InvoiceAuditActor {
	id := int64(7)
	return InvoiceAuditActor{Type: "admin", ID: &id, IPAddress: "127.0.0.1", UserAgentHash: "test-hash"}
}

type invoiceStorageReadinessStub struct{ err error }

func (s invoiceStorageReadinessStub) CheckReady(context.Context) (InvoiceStorageStatus, error) {
	return InvoiceStorageStatus{Available: s.err == nil}, s.err
}

func TestInvoiceConfigDefaultsToDisabledWithoutSetting(t *testing.T) {
	svc := NewInvoiceConfigService(&invoiceConfigSettingRepo{}, invoiceStorageReadinessStub{})
	cfg, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.False(t, cfg.Enabled)
	require.Equal(t, 0.01, cfg.MinAmount)
	require.Equal(t, 50, cfg.MaxOrdersPerRequest)
	require.Equal(t, "信息技术服务费", cfg.ItemName)
}

func TestInvoiceConfigRejectsEnableWhenStorageIsUnavailable(t *testing.T) {
	repo := &invoiceConfigAtomicSettingRepo{}
	svc := NewInvoiceConfigService(repo, invoiceStorageReadinessStub{err: errors.New("disk unavailable")})
	cfg := DefaultInvoiceConfig()
	cfg.Enabled = true

	_, err := svc.Update(context.Background(), cfg, invoiceAdminActor())
	require.Error(t, err)
	require.Equal(t, "INVOICE_STORAGE_UNAVAILABLE", infraerrors.Reason(err))
	_, persisted := repo.values[SettingKeyInvoiceConfig]
	require.False(t, persisted)
}

func TestInvoiceConfigNormalizesEmailsAndValidatesBounds(t *testing.T) {
	repo := &invoiceConfigAtomicSettingRepo{}
	svc := NewInvoiceConfigService(repo, invoiceStorageReadinessStub{})
	cfg := DefaultInvoiceConfig()
	cfg.AdminNotificationEmails = []string{"B@example.com", "a@example.com", "b@example.com"}

	updated, err := svc.Update(context.Background(), cfg, invoiceAdminActor())
	require.NoError(t, err)
	require.Equal(t, []string{"a@example.com", "b@example.com"}, updated.AdminNotificationEmails)
	require.Equal(t, 1, repo.setMultipleCalls)
	audits, err := svc.ListAudits(context.Background(), 20)
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.Equal(t, int64(7), audits[0].AdminID)
	require.Contains(t, audits[0].ChangedFields, "admin_notification_emails")
	auditRaw := repo.values[SettingKeyInvoiceConfigAudit+audits[0].ID]
	require.NotContains(t, auditRaw, "a@example.com")

	cfg.MinAmount = 10
	cfg.MaxAmount = 9
	_, err = svc.Update(context.Background(), cfg, invoiceAdminActor())
	require.Error(t, err)
	require.Equal(t, "INVOICE_CONFIG_INVALID", infraerrors.Reason(err))
}

func TestInvoiceConfigUpdateRequiresAdminActor(t *testing.T) {
	repo := &invoiceConfigAtomicSettingRepo{}
	svc := NewInvoiceConfigService(repo, invoiceStorageReadinessStub{})
	_, err := svc.Update(context.Background(), DefaultInvoiceConfig(), InvoiceAuditActor{Type: "user"})
	require.Error(t, err)
	require.Equal(t, "INVOICE_ADMIN_REQUIRED", infraerrors.Reason(err))
	require.Empty(t, repo.values)
}
