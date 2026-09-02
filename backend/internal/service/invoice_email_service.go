package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type InvoiceEmailService struct {
	notifications *NotificationEmailService
	config        *InvoiceConfigService
}

func NewInvoiceEmailService(notifications *NotificationEmailService, config *InvoiceConfigService) *InvoiceEmailService {
	return &InvoiceEmailService{notifications: notifications, config: config}
}

func (s *InvoiceEmailService) ApplicationSubmitted(ctx context.Context, request *InvoiceRequest) error {
	if s == nil || s.notifications == nil || s.config == nil || request == nil {
		return invoiceEmailUnavailable()
	}
	userVariables := invoiceEmailVariables(request, "/invoices")
	if err := s.notifications.Send(ctx, NotificationEmailSendInput{
		Event: NotificationEmailEventInvoiceSubmittedUser, RecipientEmail: request.RecipientEmail,
		RecipientName: firstInvoiceEmailName(request), UserID: request.UserID, SourceType: "invoice_request",
		SourceID: strconv.FormatInt(request.ID, 10), ReminderKey: fmt.Sprintf("revision:%d", request.Revision), Variables: userVariables,
	}); err != nil {
		return err
	}
	cfg, err := s.config.Get(ctx)
	if err != nil {
		return err
	}
	adminVariables := invoiceEmailVariables(request, "/admin/invoices")
	for _, recipient := range cfg.AdminNotificationEmails {
		if err := s.notifications.Send(ctx, NotificationEmailSendInput{
			Event: NotificationEmailEventInvoiceSubmittedAdmin, RecipientEmail: recipient,
			RecipientName: recipient, SourceType: "invoice_request", SourceID: strconv.FormatInt(request.ID, 10),
			ReminderKey: fmt.Sprintf("revision:%d", request.Revision), Variables: adminVariables,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *InvoiceEmailService) ApplicationRejected(ctx context.Context, request *InvoiceRequest) error {
	if request == nil {
		return invoiceEmailUnavailable()
	}
	return s.sendUser(ctx, NotificationEmailEventInvoiceRejected, request, fmt.Sprintf("revision:%d", request.Revision))
}

func (s *InvoiceEmailService) InvoiceIssued(ctx context.Context, request *InvoiceRequest) error {
	if request == nil {
		return invoiceEmailUnavailable()
	}
	return s.sendUser(ctx, NotificationEmailEventInvoiceIssued, request, fmt.Sprintf("revision:%d", request.Revision))
}

func (s *InvoiceEmailService) sendUser(ctx context.Context, event string, request *InvoiceRequest, reminder string) error {
	if s == nil || s.notifications == nil || request == nil {
		return invoiceEmailUnavailable()
	}
	return s.notifications.Send(ctx, NotificationEmailSendInput{
		Event: event, RecipientEmail: request.RecipientEmail, RecipientName: firstInvoiceEmailName(request),
		UserID: request.UserID, SourceType: "invoice_request", SourceID: strconv.FormatInt(request.ID, 10),
		ReminderKey: reminder, Variables: invoiceEmailVariables(request, "/invoices"),
	})
}

func invoiceEmailUnavailable() error {
	return infraerrors.Conflict("INVOICE_EMAIL_UNAVAILABLE", "invoice email service is unavailable")
}

func invoiceEmailVariables(request *InvoiceRequest, invoiceURL string) map[string]string {
	date := ""
	if request != nil && request.InvoiceDate != nil {
		date = request.InvoiceDate.Format("2006-01-02")
	}
	return map[string]string{
		"invoice_request_no":    request.RequestNo,
		"invoice_amount":        request.TotalAmount + " " + request.Currency,
		"invoice_order_count":   strconv.Itoa(request.OrderCount),
		"invoice_title":         request.TitleName,
		"invoice_status":        request.Status,
		"invoice_date":          date,
		"invoice_reject_reason": request.RejectReason,
		"invoice_url":           invoiceURL,
	}
}

func firstInvoiceEmailName(request *InvoiceRequest) string {
	if request == nil {
		return ""
	}
	if name := strings.TrimSpace(request.UserName); name != "" {
		return name
	}
	return request.UserEmail
}
