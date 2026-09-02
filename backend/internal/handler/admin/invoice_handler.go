package admin

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type InvoiceHandler struct{ service *service.InvoiceService }

func NewInvoiceHandler(invoiceService *service.InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{service: invoiceService}
}

func (h *InvoiceHandler) GetConfig(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	result, err := h.service.GetAdminConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
func (h *InvoiceHandler) UpdateConfig(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var input service.InvoiceConfig
	if c.ShouldBindJSON(&input) != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	adminID, ok := adminInvoiceSubject(c)
	if !ok {
		return
	}
	actor := service.NewInvoiceAuditActor("admin", adminID, c.ClientIP(), c.GetHeader("User-Agent"))
	result, err := h.service.UpdateAdminConfig(c.Request.Context(), input, actor)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *InvoiceHandler) ReconcileFiles(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	result, err := h.service.ReconcileFiles(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *InvoiceHandler) List(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := service.InvoiceListParams{Page: page, PageSize: pageSize, Status: c.Query("status"), Keyword: c.Query("request_no"), UserEmail: c.Query("user_email"), TitleName: c.Query("title"), OrderNo: c.Query("order_no")}
	params.CreatedFrom = parseOptionalInvoiceTime(c.Query("created_from"))
	params.CreatedTo = parseOptionalInvoiceTime(c.Query("created_to"))
	params.IssuedFrom = parseOptionalInvoiceTime(c.Query("issued_from"))
	params.IssuedTo = parseOptionalInvoiceTime(c.Query("issued_to"))
	items, total, err := h.service.ListAdmin(c.Request.Context(), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}
func (h *InvoiceHandler) Get(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := parseAdminInvoiceID(c)
	if !ok {
		return
	}
	result, err := h.service.GetAdmin(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type invoiceAdminActionRequest struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

func (h *InvoiceHandler) Accept(c *gin.Context) {
	h.runAction(c, "invoice.accept", func(ctx context.Context, p service.InvoiceAdminActionParams) (any, error) {
		return h.service.Accept(ctx, p)
	})
}
func (h *InvoiceHandler) Release(c *gin.Context) {
	h.runAction(c, "invoice.release", func(ctx context.Context, p service.InvoiceAdminActionParams) (any, error) {
		return h.service.Release(ctx, p)
	})
}
func (h *InvoiceHandler) Reject(c *gin.Context) {
	h.runAction(c, "invoice.reject", func(ctx context.Context, p service.InvoiceAdminActionParams) (any, error) {
		return h.service.Reject(ctx, p)
	})
}
func (h *InvoiceHandler) Void(c *gin.Context) {
	h.runAction(c, "invoice.void", func(ctx context.Context, p service.InvoiceAdminActionParams) (any, error) {
		return h.service.Void(ctx, p)
	})
}

func (h *InvoiceHandler) runAction(c *gin.Context, scope string, execute func(context.Context, service.InvoiceAdminActionParams) (any, error)) {
	if !h.requireService(c) {
		return
	}
	id, ok := parseAdminInvoiceID(c)
	if !ok {
		return
	}
	var input invoiceAdminActionRequest
	if c.ShouldBindJSON(&input) != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	adminID, ok := adminInvoiceSubject(c)
	if !ok {
		return
	}
	actor := service.NewInvoiceAuditActor("admin", adminID, c.ClientIP(), c.GetHeader("User-Agent"))
	params := service.InvoiceAdminActionParams{RequestID: id, AdminID: adminID, Reason: input.Reason, Note: input.Note, Actor: actor}
	fingerprint := struct {
		ID    int64                     `json:"id"`
		Input invoiceAdminActionRequest `json:"input"`
	}{ID: id, Input: input}
	executeAdminIdempotentJSON(c, scope, fingerprint, 24*time.Hour, func(ctx context.Context) (any, error) { return execute(ctx, params) })
}

func (h *InvoiceHandler) Issue(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := parseAdminInvoiceID(c)
	if !ok {
		return
	}
	adminID, ok := adminInvoiceSubject(c)
	if !ok {
		return
	}
	source, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "Invoice file is required")
		return
	}
	defer source.Close()
	date, err := time.Parse("2006-01-02", strings.TrimSpace(c.PostForm("invoice_date")))
	if err != nil {
		response.BadRequest(c, "Invoice date is invalid")
		return
	}
	actor := service.NewInvoiceAuditActor("admin", adminID, c.ClientIP(), c.GetHeader("User-Agent"))
	result, err := h.service.Issue(c.Request.Context(), service.InvoiceIssueParams{RequestID: id, AdminID: adminID, InvoiceCode: c.PostForm("invoice_code"), InvoiceNumber: c.PostForm("invoice_number"), InvoiceDate: date, Actor: actor}, service.InvoiceUploadInput{Filename: header.Filename, Reader: source})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *InvoiceHandler) ReplaceFile(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := parseAdminInvoiceID(c)
	if !ok {
		return
	}
	adminID, ok := adminInvoiceSubject(c)
	if !ok {
		return
	}
	source, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "Invoice file is required")
		return
	}
	defer source.Close()
	date, err := time.Parse("2006-01-02", strings.TrimSpace(c.PostForm("invoice_date")))
	if err != nil {
		response.BadRequest(c, "Invoice date is invalid")
		return
	}
	actor := service.NewInvoiceAuditActor("admin", adminID, c.ClientIP(), c.GetHeader("User-Agent"))
	result, err := h.service.ReplaceFile(c.Request.Context(), service.InvoiceReplaceFileParams{RequestID: id, AdminID: adminID, Reason: c.PostForm("reason"), InvoiceDate: date, Actor: actor}, service.InvoiceUploadInput{Filename: header.Filename, Reader: source})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *InvoiceHandler) Download(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := parseAdminInvoiceID(c)
	if !ok {
		return
	}
	download, err := h.service.DownloadAdmin(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	defer download.Reader.Close()
	writeAdminInvoiceDownload(c, download)
}
func (h *InvoiceHandler) ListAuditLogs(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := parseAdminInvoiceID(c)
	if !ok {
		return
	}
	items, err := h.service.ListAuditLogs(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}
func (h *InvoiceHandler) ResendEmail(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := parseAdminInvoiceID(c)
	if !ok {
		return
	}
	payload := struct {
		ID int64 `json:"id"`
	}{id}
	executeAdminIdempotentJSON(c, "invoice.resend_email", payload, 24*time.Hour, func(ctx context.Context) (any, error) { return h.service.ResendEmail(ctx, id) })
}

func adminInvoiceSubject(c *gin.Context) (int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Authentication required")
		return 0, false
	}
	return subject.UserID, true
}
func parseAdminInvoiceID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid invoice request ID")
		return 0, false
	}
	return id, true
}
func parseOptionalInvoiceTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}
func writeAdminInvoiceDownload(c *gin.Context, download *service.InvoiceDownload) {
	if download == nil || download.Reader == nil {
		response.NotFound(c, "Invoice file not found")
		return
	}
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": download.Metadata.OriginalFilename}))
	c.Header("Content-Type", download.Metadata.ContentType)
	c.Header("Content-Length", strconv.FormatInt(download.Metadata.FileSize, 10))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, download.Reader); err != nil {
		_ = c.Error(fmt.Errorf("stream invoice download: %w", err))
	}
}

func (h *InvoiceHandler) requireService(c *gin.Context) bool {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("INVOICE_SERVICE_UNAVAILABLE", "invoice service is unavailable"))
		return false
	}
	return true
}
