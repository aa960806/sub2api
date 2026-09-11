package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ModelEvaluationAdminService interface {
	GetConfig(context.Context) (*service.ModelEvaluationConfig, error)
	UpdateConfig(context.Context, service.ModelEvaluationConfig) (*service.ModelEvaluationConfig, error)
	ListTasks(context.Context) ([]*service.ModelEvaluationTask, error)
	CreateTask(context.Context, service.ModelEvaluationTaskInput) (*service.ModelEvaluationTask, error)
	UpdateTask(context.Context, int64, service.ModelEvaluationTaskInput) (*service.ModelEvaluationTask, error)
	DeleteTask(context.Context, int64) error
	RunNow(context.Context, int64) error
	TestTask(context.Context, int64) error
	SetPublication(context.Context, int64, bool) (*service.ModelEvaluationTask, error)
	ListResults(context.Context, service.ModelEvaluationListParams) ([]*service.ModelEvaluationResult, int64, error)
	GetResult(context.Context, int64, service.ModelEvaluationListParams) (*service.ModelEvaluationResult, error)
	DeleteResult(context.Context, int64) error
	Cleanup(context.Context, service.ModelEvaluationCleanupParams) (int64, error)
}

type ModelEvaluationHandler struct {
	service  ModelEvaluationAdminService
	settings *service.SettingService
}

func NewModelEvaluationHandler(s *service.ModelEvaluationService, settings *service.SettingService) *ModelEvaluationHandler {
	return &ModelEvaluationHandler{service: s, settings: settings}
}

// Request parsing never includes input values in errors: bodies may contain a key.
func BindModelEvaluationJSON(c *gin.Context, dst any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		response.BadRequest(c, "Invalid model evaluation request")
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		response.BadRequest(c, "Invalid model evaluation request")
		return false
	}
	return true
}

func ParseModelEvaluationID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid ID")
		return 0, false
	}
	return id, true
}

func ParseModelEvaluationList(c *gin.Context) (service.ModelEvaluationListParams, bool) {
	p := service.ModelEvaluationListParams{Page: 1, PageSize: 12}
	for name, target := range map[string]*int64{"group_id": &p.GroupID, "task_id": &p.TaskID} {
		if raw, exists := c.GetQuery(name); exists {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < 1 {
				response.BadRequest(c, "Invalid filter")
				return p, false
			}
			*target = value
		}
	}
	for name, target := range map[string]*int{"page": &p.Page, "page_size": &p.PageSize} {
		if raw, exists := c.GetQuery(name); exists {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || (name == "page" && value > 100000) || (name == "page_size" && value > 100) {
				response.BadRequest(c, "Invalid pagination")
				return p, false
			}
			*target = value
		}
	}
	return p, true
}

func (h *ModelEvaluationHandler) GetConfig(c *gin.Context) {
	cfg, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}
func (h *ModelEvaluationHandler) UpdateConfig(c *gin.Context) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if !BindModelEvaluationJSON(c, &req) {
		return
	}
	if req.Enabled == nil {
		response.BadRequest(c, "Enabled is required")
		return
	}
	cfg, err := h.service.UpdateConfig(c.Request.Context(), service.ModelEvaluationConfig{Enabled: *req.Enabled})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.settings != nil {
		h.settings.NotifySettingsUpdated()
	}
	response.Success(c, cfg)
}
func (h *ModelEvaluationHandler) ListTasks(c *gin.Context) {
	items, err := h.service.ListTasks(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if items == nil {
		items = []*service.ModelEvaluationTask{}
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"items": items})
}
func (h *ModelEvaluationHandler) CreateTask(c *gin.Context) {
	var req service.ModelEvaluationTaskInput
	if !BindModelEvaluationJSON(c, &req) {
		return
	}
	item, err := h.service.CreateTask(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, item)
}
func (h *ModelEvaluationHandler) UpdateTask(c *gin.Context) {
	id, ok := ParseModelEvaluationID(c)
	if !ok {
		return
	}
	var req service.ModelEvaluationTaskInput
	if !BindModelEvaluationJSON(c, &req) {
		return
	}
	item, err := h.service.UpdateTask(c.Request.Context(), id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}
func (h *ModelEvaluationHandler) DeleteTask(c *gin.Context) {
	id, ok := ParseModelEvaluationID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteTask(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}
func (h *ModelEvaluationHandler) RunNow(c *gin.Context) {
	id, ok := ParseModelEvaluationID(c)
	if !ok {
		return
	}
	if err := h.service.RunNow(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Accepted(c, gin.H{"queued": true})
}

func (h *ModelEvaluationHandler) TestTask(c *gin.Context) {
	id, ok := ParseModelEvaluationID(c)
	if !ok {
		return
	}
	if err := h.service.TestTask(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Accepted(c, gin.H{"queued": true})
}

func (h *ModelEvaluationHandler) SetPublication(c *gin.Context) {
	id, ok := ParseModelEvaluationID(c)
	if !ok {
		return
	}
	var req struct {
		Published *bool `json:"published"`
	}
	if !BindModelEvaluationJSON(c, &req) {
		return
	}
	if req.Published == nil {
		response.BadRequest(c, "Published is required")
		return
	}
	task, err := h.service.SetPublication(c.Request.Context(), id, *req.Published)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, task)
}
func (h *ModelEvaluationHandler) ListResults(c *gin.Context) {
	p, ok := ParseModelEvaluationList(c)
	if !ok {
		return
	}
	p.Admin = true
	items, total, err := h.service.ListResults(c.Request.Context(), p)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if items == nil {
		items = []*service.ModelEvaluationResult{}
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"items": items, "total": total, "page": p.Page, "page_size": p.PageSize})
}
func (h *ModelEvaluationHandler) GetResult(c *gin.Context) {
	id, ok := ParseModelEvaluationID(c)
	if !ok {
		return
	}
	item, err := h.service.GetResult(c.Request.Context(), id, service.ModelEvaluationListParams{Admin: true})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	response.Success(c, item)
}
func (h *ModelEvaluationHandler) DeleteResult(c *gin.Context) {
	id, ok := ParseModelEvaluationID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteResult(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}
func (h *ModelEvaluationHandler) Cleanup(c *gin.Context) {
	var req service.ModelEvaluationCleanupParams
	if !BindModelEvaluationJSON(c, &req) {
		return
	}
	n, err := h.service.Cleanup(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": n})
}
