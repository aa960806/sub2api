package handler

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type InvoiceHandler struct {
	service *service.InvoiceService
}

func NewInvoiceHandler(invoiceService *service.InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{service: invoiceService}
}

func (h *InvoiceHandler) GetConfig(c *gin.Context) {
	subject, ok := requireInvoiceSubject(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("INVOICE_SERVICE_UNAVAILABLE", "invoice service is unavailable"))
		return
	}
	result, err := h.service.GetPublicConfig(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *InvoiceHandler) ListEligibleOrders(c *gin.Context) {
	subject, ok := requireInvoiceSubject(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("INVOICE_SERVICE_UNAVAILABLE", "invoice service is unavailable"))
		return
	}
	page, pageSize := response.ParsePagination(c)
	result, err := h.service.ListEligibleOrders(c.Request.Context(), subject.UserID, page, pageSize, c.Query("keyword"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *InvoiceHandler) ListMy(c *gin.Context) {
	subject, ok := requireInvoiceSubject(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("INVOICE_SERVICE_UNAVAILABLE", "invoice service is unavailable"))
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListMy(c.Request.Context(), subject.UserID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *InvoiceHandler) GetMy(c *gin.Context) {
	subject, ok := requireInvoiceSubject(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("INVOICE_SERVICE_UNAVAILABLE", "invoice service is unavailable"))
		return
	}
	id, ok := parseInvoiceID(c)
	if !ok {
		return
	}
	result, err := h.service.GetMy(c.Request.Context(), id, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *InvoiceHandler) Create(c *gin.Context) {
	subject, ok := requireInvoiceSubject(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("INVOICE_SERVICE_UNAVAILABLE", "invoice service is unavailable"))
		return
	}
	var input service.InvoiceCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	actor := service.NewInvoiceAuditActor("user", subject.UserID, c.ClientIP(), c.GetHeader("User-Agent"))
	executeUserIdempotentJSON(c, "invoice.create", input, 24*time.Hour, func(ctx context.Context) (any, error) {
		return h.service.Create(ctx, subject.UserID, input, actor)
	})
}

func (h *InvoiceHandler) Cancel(c *gin.Context) {
	subject, ok := requireInvoiceSubject(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("INVOICE_SERVICE_UNAVAILABLE", "invoice service is unavailable"))
		return
	}
	id, ok := parseInvoiceID(c)
	if !ok {
		return
	}
	payload := struct {
		ID int64 `json:"id"`
	}{ID: id}
	actor := service.NewInvoiceAuditActor("user", subject.UserID, c.ClientIP(), c.GetHeader("User-Agent"))
	executeUserIdempotentJSON(c, "invoice.cancel", payload, 24*time.Hour, func(ctx context.Context) (any, error) {
		return h.service.Cancel(ctx, id, subject.UserID, actor)
	})
}

func (h *InvoiceHandler) Resubmit(c *gin.Context) {
	subject, ok := requireInvoiceSubject(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("INVOICE_SERVICE_UNAVAILABLE", "invoice service is unavailable"))
		return
	}
	id, ok := parseInvoiceID(c)
	if !ok {
		return
	}
	var input service.InvoiceResubmitInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	actor := service.NewInvoiceAuditActor("user", subject.UserID, c.ClientIP(), c.GetHeader("User-Agent"))
	fingerprint := struct {
		ID    int64                        `json:"id"`
		Input service.InvoiceResubmitInput `json:"input"`
	}{ID: id, Input: input}
	executeUserIdempotentJSON(c, "invoice.resubmit", fingerprint, 24*time.Hour, func(ctx context.Context) (any, error) {
		return h.service.Resubmit(ctx, id, subject.UserID, input, actor)
	})
}

func (h *InvoiceHandler) Download(c *gin.Context) {
	subject, ok := requireInvoiceSubject(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("INVOICE_SERVICE_UNAVAILABLE", "invoice service is unavailable"))
		return
	}
	id, ok := parseInvoiceID(c)
	if !ok {
		return
	}
	download, err := h.service.DownloadMy(c.Request.Context(), id, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	defer download.Reader.Close()
	writeInvoiceDownload(c, download)
}

func requireInvoiceSubject(c *gin.Context) (middleware2.AuthSubject, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Authentication required")
		return middleware2.AuthSubject{}, false
	}
	return subject, true
}

func parseInvoiceID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid invoice request ID")
		return 0, false
	}
	return id, true
}

func writeInvoiceDownload(c *gin.Context, download *service.InvoiceDownload) {
	if download == nil || download.Reader == nil {
		response.NotFound(c, "Invoice file not found")
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": download.Metadata.OriginalFilename})
	c.Header("Content-Disposition", disposition)
	c.Header("Content-Type", download.Metadata.ContentType)
	c.Header("Content-Length", strconv.FormatInt(download.Metadata.FileSize, 10))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, download.Reader); err != nil {
		_ = c.Error(fmt.Errorf("stream invoice download: %w", err))
	}
}
