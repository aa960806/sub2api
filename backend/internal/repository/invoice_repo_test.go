package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestValidateInvoiceOrdersUsesDecimalAndRefundClosure(t *testing.T) {
	now := time.Now()
	orders := []lockedPaymentOrder{
		{ID: 1, OrderType: payment.OrderTypeBalance, PayAmount: "0.10", ProviderSnapshot: `{}`, Status: payment.OrderStatusCompleted, RefundAmount: "0", CompletedAt: validInvoiceNullTime(now)},
		{ID: 2, OrderType: payment.OrderTypeSubscription, PayAmount: "0.20", ProviderSnapshot: `{"currency":"CNY"}`, Status: payment.OrderStatusCompleted, RefundAmount: "0.00", PaidAt: validInvoiceNullTime(now)},
	}
	total, err := validateInvoiceOrders(orders, service.InvoiceConfig{MinAmount: 0.01}, now)
	require.NoError(t, err)
	require.Equal(t, "0.30", total.StringFixed(2))

	orders[1].Status = payment.OrderStatusRefundFailed
	_, err = validateInvoiceOrders(orders, service.InvoiceConfig{MinAmount: 0.01}, now)
	require.Equal(t, "ORDER_NOT_ELIGIBLE", infraerrors.Reason(err))
}

func TestValidateInvoiceOrdersRejectsEveryRefundStatus(t *testing.T) {
	refundStatuses := []string{
		payment.OrderStatusRefundRequested,
		payment.OrderStatusRefunding,
		payment.OrderStatusRefundPending,
		payment.OrderStatusPartiallyRefunded,
		payment.OrderStatusRefunded,
		payment.OrderStatusRefundFailed,
	}
	for _, status := range refundStatuses {
		t.Run(status, func(t *testing.T) {
			orders := []lockedPaymentOrder{{
				ID: 1, OrderType: payment.OrderTypeBalance, PayAmount: "1.00", ProviderSnapshot: `{}`,
				Status: status, RefundAmount: "0", CompletedAt: validInvoiceNullTime(time.Now()),
			}}
			_, err := validateInvoiceOrders(orders, service.InvoiceConfig{MinAmount: 0.01}, time.Now())
			require.Equal(t, "ORDER_NOT_ELIGIBLE", infraerrors.Reason(err))
		})
	}
}

func TestInvoiceReleaseRejectsOversizedReasonBeforeDatabaseAccess(t *testing.T) {
	repo := &invoiceRepository{}
	_, err := repo.Release(context.Background(), service.InvoiceAdminActionParams{
		Reason: strings.Repeat("超", 1001),
	})
	require.Equal(t, "INVALID_INVOICE_STATUS_TRANSITION", infraerrors.Reason(err))
}

func TestInvoiceAdminTransitionRejectsOversizedEffectiveNoteBeforeDatabaseAccess(t *testing.T) {
	tests := []struct {
		name   string
		note   string
		reason string
	}{
		{name: "note takes precedence", note: strings.Repeat("超", 1001), reason: "short reason"},
		{name: "reason fallback", note: "   ", reason: strings.Repeat("超", 1001)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &invoiceRepository{}
			_, err := repo.Accept(context.Background(), service.InvoiceAdminActionParams{Note: tt.note, Reason: tt.reason})
			require.Equal(t, "INVALID_INVOICE_STATUS_TRANSITION", infraerrors.Reason(err))
		})
	}
}

func TestValidateInvoiceOrdersUsesCanonicalDecimalConfigBounds(t *testing.T) {
	now := time.Now()
	orders := []lockedPaymentOrder{{
		ID: 1, OrderType: payment.OrderTypeBalance, PayAmount: "0.29", ProviderSnapshot: `{}`,
		Status: payment.OrderStatusCompleted, RefundAmount: "0", CompletedAt: validInvoiceNullTime(now),
	}}

	total, err := validateInvoiceOrders(orders, service.InvoiceConfig{MinAmount: 0.29, MaxAmount: 0.29}, now)
	require.NoError(t, err)
	require.Equal(t, "0.29", total.StringFixed(2))

	_, err = validateInvoiceOrders(orders, service.InvoiceConfig{MinAmount: 0.30}, now)
	require.Equal(t, "INVOICE_AMOUNT_BELOW_MINIMUM", infraerrors.Reason(err))
	_, err = validateInvoiceOrders(orders, service.InvoiceConfig{MinAmount: 0.01, MaxAmount: 0.28}, now)
	require.Equal(t, "INVOICE_AMOUNT_ABOVE_MAXIMUM", infraerrors.Reason(err))
}

func TestPaymentOrderCurrencyFailsClosedForExplicitInvalidValues(t *testing.T) {
	tests := map[string]string{
		`{}`:                   payment.DefaultPaymentCurrency,
		`{"currency":null}`:    payment.DefaultPaymentCurrency,
		`{"currency":""}`:      payment.DefaultPaymentCurrency,
		`{"currency":" cny "}`: payment.DefaultPaymentCurrency,
		`{"currency":"EUR"}`:   "EUR",
		`{"currency":"US1"}`:   "",
		`{"currency":123}`:     "",
		`not-json`:             "",
	}
	for raw, expected := range tests {
		require.Equal(t, expected, paymentOrderCurrency(raw), raw)
	}
}

func TestNormalizeInvoiceOrderIDsRejectsDuplicatesAndSorts(t *testing.T) {
	ids, err := normalizeInvoiceOrderIDs([]int64{9, 2, 5}, 50)
	require.NoError(t, err)
	require.Equal(t, []int64{2, 5, 9}, ids)
	_, err = normalizeInvoiceOrderIDs([]int64{2, 2}, 50)
	require.Equal(t, "INVALID_ORDER_SELECTION", infraerrors.Reason(err))
}

func TestInvoiceIssueReplayMatchesExactCommittedResult(t *testing.T) {
	date := time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
	request := &service.InvoiceRequest{
		Status: service.InvoiceStatusIssued, InvoiceCode: "CODE", InvoiceNumber: "NUMBER", InvoiceDate: &date,
	}
	params := service.InvoiceIssueParams{
		InvoiceCode: "CODE", InvoiceNumber: "NUMBER", InvoiceDate: date,
		File: service.InvoiceFileMetadata{SHA256: "abc123"},
	}
	require.True(t, invoiceIssueReplayMatches(request, params, "abc123"))

	params.InvoiceNumber = "OTHER"
	require.False(t, invoiceIssueReplayMatches(request, params, "abc123"))
	params.InvoiceNumber = "NUMBER"
	require.False(t, invoiceIssueReplayMatches(request, params, "different"))
}

func validInvoiceNullTime(value time.Time) sql.NullTime {
	return sql.NullTime{Time: value, Valid: true}
}
