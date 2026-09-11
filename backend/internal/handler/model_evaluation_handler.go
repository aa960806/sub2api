package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type modelEvaluationUserService interface {
	GetConfig(context.Context) (*service.ModelEvaluationConfig, error)
	ListGroups(context.Context, []int64) ([]service.ModelEvaluationGroup, error)
	ListResults(context.Context, service.ModelEvaluationListParams) ([]*service.ModelEvaluationResult, int64, error)
	GetResult(context.Context, int64, service.ModelEvaluationListParams) (*service.ModelEvaluationResult, error)
}
type modelEvaluationGroupAccess interface {
	GetAvailableGroups(context.Context, int64) ([]service.Group, error)
}
type ModelEvaluationUserHandler struct {
	service modelEvaluationUserService
	groups  modelEvaluationGroupAccess
}

func NewModelEvaluationUserHandler(s *service.ModelEvaluationService, keys *service.APIKeyService) *ModelEvaluationUserHandler {
	return &ModelEvaluationUserHandler{service: s, groups: keys}
}
func (h *ModelEvaluationUserHandler) scope(c *gin.Context) (service.ModelEvaluationListParams, bool, bool) {
	p := service.ModelEvaluationListParams{AllowedGroupIDs: []int64{}}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return p, false, false
	}
	cfg, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return p, false, false
	}
	if cfg == nil || !cfg.Enabled {
		return p, false, true
	}
	groups, err := h.groups.GetAvailableGroups(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return p, false, false
	}
	for _, group := range groups {
		p.AllowedGroupIDs = append(p.AllowedGroupIDs, group.ID)
	}
	return p, true, true
}
func (h *ModelEvaluationUserHandler) Groups(c *gin.Context) {
	p, enabled, ok := h.scope(c)
	if !ok {
		return
	}
	items := []service.ModelEvaluationGroup{}
	if enabled {
		var err error
		items, err = h.service.ListGroups(c.Request.Context(), p.AllowedGroupIDs)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if items == nil {
			items = []service.ModelEvaluationGroup{}
		}
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"enabled": enabled, "items": items})
}
func (h *ModelEvaluationUserHandler) ListResults(c *gin.Context) {
	scope, enabled, ok := h.scope(c)
	if !ok {
		return
	}
	p, ok := admin.ParseModelEvaluationList(c)
	if !ok {
		return
	}
	p.AllowedGroupIDs = scope.AllowedGroupIDs
	items := []*service.ModelEvaluationResult{}
	var total int64
	if enabled {
		var err error
		items, total, err = h.service.ListResults(c.Request.Context(), p)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if items == nil {
			items = []*service.ModelEvaluationResult{}
		}
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"items": items, "total": total, "page": p.Page, "page_size": p.PageSize})
}
func (h *ModelEvaluationUserHandler) GetResult(c *gin.Context) {
	p, enabled, ok := h.scope(c)
	if !ok {
		return
	}
	if !enabled {
		response.ErrorFrom(c, service.ErrModelEvaluationNotFound)
		return
	}
	id, ok := admin.ParseModelEvaluationID(c)
	if !ok {
		return
	}
	item, err := h.service.GetResult(c.Request.Context(), id, p)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	response.Success(c, item)
}
