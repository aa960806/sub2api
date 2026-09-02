package admin

import (
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type SubNexusMarqueeHandler struct {
	service *service.MarqueeService
}

func NewSubNexusMarqueeHandler(marqueeService *service.MarqueeService) *SubNexusMarqueeHandler {
	return &SubNexusMarqueeHandler{service: marqueeService}
}

func (h *SubNexusMarqueeHandler) GetConfig(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_MARQUEE_UNAVAILABLE", "marquee service is unavailable"))
		return
	}
	response.Success(c, h.service.GetConfig(c.Request.Context()))
}

func (h *SubNexusMarqueeHandler) UpdateConfig(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_MARQUEE_UNAVAILABLE", "marquee service is unavailable"))
		return
	}
	var req service.MarqueeConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.service.UpdateConfig(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *SubNexusMarqueeHandler) List(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_MARQUEE_UNAVAILABLE", "marquee service is unavailable"))
		return
	}
	items, err := h.service.ListAdmin(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *SubNexusMarqueeHandler) Create(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_MARQUEE_UNAVAILABLE", "marquee service is unavailable"))
		return
	}
	var req service.MarqueeBroadcastInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	adminID := int64(0)
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok {
		adminID = subject.UserID
	}
	item, err := h.service.Create(c.Request.Context(), req, adminID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *SubNexusMarqueeHandler) Update(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_MARQUEE_UNAVAILABLE", "marquee service is unavailable"))
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid broadcast id")
		return
	}
	var req service.MarqueeBroadcastInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.service.Update(c.Request.Context(), id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *SubNexusMarqueeHandler) Delete(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_MARQUEE_UNAVAILABLE", "marquee service is unavailable"))
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid broadcast id")
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}
