package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type invoiceRepositoryGetAdminStub struct {
	InvoiceRepository
	request *InvoiceRequest
	err     error
}

func (s *invoiceRepositoryGetAdminStub) GetAdmin(context.Context, int64) (*InvoiceRequest, error) {
	return s.request, s.err
}

type invoiceRepositoryCancelStub struct {
	InvoiceRepository
	called  bool
	request *InvoiceRequest
	err     error
}

type invoiceEmailNotifierStub struct {
	issuedCalls int
}

func (*invoiceEmailNotifierStub) ApplicationSubmitted(context.Context, *InvoiceRequest) error {
	return nil
}

func (*invoiceEmailNotifierStub) ApplicationRejected(context.Context, *InvoiceRequest) error {
	return nil
}

func (s *invoiceEmailNotifierStub) InvoiceIssued(context.Context, *InvoiceRequest) error {
	s.issuedCalls++
	return nil
}

func (s *invoiceRepositoryCancelStub) Cancel(context.Context, int64, int64, InvoiceAuditActor) (*InvoiceRequest, error) {
	s.called = true
	return s.request, s.err
}

func TestInvoiceCancelIsBlockedWhenFeatureIsDisabled(t *testing.T) {
	repo := &invoiceRepositoryCancelStub{request: &InvoiceRequest{ID: 1, Status: InvoiceStatusCancelled}}
	settings := &invoiceConfigSettingRepo{values: map[string]string{
		SettingKeyInvoiceConfig: `{"enabled":false}`,
	}}
	svc := NewInvoiceService(repo, NewInvoiceConfigService(settings, nil), nil, nil)

	result, err := svc.Cancel(context.Background(), 1, 7, NewInvoiceAuditActor("user", 7, "127.0.0.1", "test"))
	require.Nil(t, result)
	require.Equal(t, "INVOICE_DISABLED", infraerrors.Reason(err))
	require.False(t, repo.called, "disabled cancellation must not reach the repository")
}

func TestInvoiceCancelDelegatesWhenFeatureIsEnabled(t *testing.T) {
	repo := &invoiceRepositoryCancelStub{request: &InvoiceRequest{ID: 1, Status: InvoiceStatusCancelled}}
	settings := &invoiceConfigSettingRepo{values: map[string]string{
		SettingKeyInvoiceConfig:          `{"enabled":true}`,
		SettingKeySubNexusInvoiceEnabled: "true",
	}}
	svc := NewInvoiceService(repo, NewInvoiceConfigService(settings, nil), nil, nil)

	result, err := svc.Cancel(context.Background(), 1, 7, NewInvoiceAuditActor("user", 7, "127.0.0.1", "test"))
	require.NoError(t, err)
	require.Same(t, repo.request, result)
	require.True(t, repo.called)
}

func TestInvoiceResendDoesNotMislabelVoidedInvoiceAsIssued(t *testing.T) {
	repo := &invoiceRepositoryGetAdminStub{request: &InvoiceRequest{ID: 1, Status: InvoiceStatusVoided}}
	settings := &invoiceConfigSettingRepo{values: map[string]string{
		SettingKeyInvoiceConfig:          `{"enabled":true}`,
		SettingKeySubNexusInvoiceEnabled: "true",
	}}
	notifier := &invoiceEmailNotifierStub{}
	svc := NewInvoiceService(repo, NewInvoiceConfigService(settings, nil), nil, notifier)

	result, err := svc.ResendEmail(context.Background(), 1)
	require.Same(t, repo.request, result)
	require.Equal(t, "INVALID_INVOICE_STATUS_TRANSITION", infraerrors.Reason(err))
	require.Zero(t, notifier.issuedCalls)
}

func TestInvoiceEmailNotifierFailsClosedWhenDependencyIsMissing(t *testing.T) {
	var notifier *InvoiceEmailService
	err := notifier.InvoiceIssued(context.Background(), &InvoiceRequest{ID: 1})
	require.Equal(t, "INVOICE_EMAIL_UNAVAILABLE", infraerrors.Reason(err))
	err = (&InvoiceEmailService{}).ApplicationRejected(context.Background(), nil)
	require.Equal(t, "INVOICE_EMAIL_UNAVAILABLE", infraerrors.Reason(err))
}

func TestInvoiceEmailVariablesKeepUserAndAdminDestinationsSeparate(t *testing.T) {
	request := &InvoiceRequest{RequestNo: "INV-1"}
	require.Equal(t, "/invoices", invoiceEmailVariables(request, "/invoices")["invoice_url"])
	require.Equal(t, "/admin/invoices", invoiceEmailVariables(request, "/admin/invoices")["invoice_url"])
}

func TestNormalizeAndValidateInvoiceHeader(t *testing.T) {
	company, err := normalizeAndValidateInvoiceHeader(InvoiceHeaderInput{
		TitleType: "company", TitleName: " Example Co. ", TaxpayerID: "91350100M000100Y43",
		RecipientEmail: "Billing@Example.com", CompanyAddress: "Address",
	})
	require.NoError(t, err)
	require.Equal(t, InvoiceTitleCompany, company.TitleType)
	require.Equal(t, "billing@example.com", company.RecipientEmail)
	require.Equal(t, "91350100M000100Y43", company.TaxpayerID)

	personal, err := normalizeAndValidateInvoiceHeader(InvoiceHeaderInput{
		TitleType: "personal", TitleName: "Alex", RecipientEmail: "alex@example.com",
		CompanyAddress: "must be cleared",
	})
	require.NoError(t, err)
	require.Empty(t, personal.CompanyAddress)

	_, err = normalizeAndValidateInvoiceHeader(InvoiceHeaderInput{
		TitleType: "personal", TitleName: "Alex", TaxpayerID: "91350100M000100Y43", RecipientEmail: "alex@example.com",
	})
	require.Equal(t, "INVALID_INVOICE_HEADER", infraerrors.Reason(err))
}

func TestNormalizeInvoiceDateRejectsFutureDate(t *testing.T) {
	_, err := normalizeInvoiceDate(time.Now().UTC().Add(72 * time.Hour))
	require.Equal(t, "INVALID_INVOICE_HEADER", infraerrors.Reason(err))
	date, err := normalizeInvoiceDate(time.Now())
	require.NoError(t, err)
	require.Zero(t, date.Hour())
}

func TestSanitizeInvoiceRequestForUserRemovesAdminOnlyFields(t *testing.T) {
	adminID := int64(9)
	request := &InvoiceRequest{
		AdminNote: "internal only", AcceptedBy: &adminID, IssuedBy: &adminID,
		ConfigSnapshot: InvoiceConfig{AdminNotificationEmails: []string{"finance@example.com"}, MaxFileSizeMB: 10},
		CurrentFile:    &InvoiceFileMetadata{UploadedBy: adminID},
	}
	sanitizeInvoiceRequestForUser(request)
	require.Empty(t, request.AdminNote)
	require.Nil(t, request.AcceptedBy)
	require.Nil(t, request.IssuedBy)
	require.Empty(t, request.ConfigSnapshot.AdminNotificationEmails)
	require.Zero(t, request.CurrentFile.UploadedBy)
}

func TestMaskInvoiceIdentifierKeepsOnlyLastFourRunes(t *testing.T) {
	require.Equal(t, "**************0Y43", maskInvoiceIdentifier("91350100M000100Y43"))
	require.Equal(t, "****", maskInvoiceIdentifier("1234"))
	require.Empty(t, maskInvoiceIdentifier(""))
}

func TestResolvePreparedFileResultDeletesOnlyConfirmedOrphans(t *testing.T) {
	persistenceErr := errors.New("persistence result unavailable")

	t.Run("successful duplicate removes unreferenced prepared file", func(t *testing.T) {
		service, prepared, path := newInvoicePreparedResultFixture(t, &InvoiceRequest{
			CurrentFile: &InvoiceFileMetadata{StorageKey: "existing.pdf"},
		}, nil)

		result, err := service.resolvePreparedFileResult(context.Background(), prepared, service.repo.(*invoiceRepositoryGetAdminStub).request, nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NoFileExists(t, path)
	})

	t.Run("committed file survives a post-commit response error", func(t *testing.T) {
		service, prepared, path := newInvoicePreparedResultFixture(t, nil, nil)
		service.repo.(*invoiceRepositoryGetAdminStub).request = &InvoiceRequest{
			CurrentFile: &InvoiceFileMetadata{StorageKey: prepared.Metadata.StorageKey},
		}

		result, err := service.resolvePreparedFileResult(context.Background(), prepared, nil, persistenceErr)
		require.NoError(t, err)
		require.Equal(t, prepared.Metadata.StorageKey, result.CurrentFile.StorageKey)
		require.FileExists(t, path)
	})

	t.Run("confirmed database failure removes orphan", func(t *testing.T) {
		service, prepared, path := newInvoicePreparedResultFixture(t, &InvoiceRequest{}, nil)

		result, err := service.resolvePreparedFileResult(context.Background(), prepared, nil, persistenceErr)
		require.Nil(t, result)
		require.ErrorIs(t, err, persistenceErr)
		require.NoFileExists(t, path)
	})

	t.Run("uncertain database state preserves file for reconciliation", func(t *testing.T) {
		lookupErr := errors.New("database unavailable")
		service, prepared, path := newInvoicePreparedResultFixture(t, nil, lookupErr)

		result, err := service.resolvePreparedFileResult(context.Background(), prepared, nil, persistenceErr)
		require.Nil(t, result)
		require.ErrorIs(t, err, persistenceErr)
		require.FileExists(t, path)
	})
}

func newInvoicePreparedResultFixture(t *testing.T, request *InvoiceRequest, lookupErr error) (*InvoiceService, *InvoicePreparedFile, string) {
	t.Helper()
	root := t.TempDir()
	storageKey := filepath.ToSlash(filepath.Join("2026", "08", "11", "prepared.pdf"))
	path := filepath.Join(root, filepath.FromSlash(storageKey))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("%PDF-1.7\n%%EOF"), 0o600))
	prepared := &InvoicePreparedFile{Metadata: InvoiceFileMetadata{
		InvoiceRequestID: 11,
		StorageKey:       storageKey,
	}}
	repo := &invoiceRepositoryGetAdminStub{request: request, err: lookupErr}
	return &InvoiceService{repo: repo, files: &InvoiceFileStore{root: root}}, prepared, path
}
