package handler

import (
	"strconv"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type SubNexusMarqueeHandler struct {
	service *service.MarqueeService
}

func NewSubNexusMarqueeHandler(marqueeService *service.MarqueeService) *SubNexusMarqueeHandler {
	return &SubNexusMarqueeHandler{service: marqueeService}
}

func (h *SubNexusMarqueeHandler) List(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_MARQUEE_UNAVAILABLE", "marquee service is unavailable"))
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "12"))
	result, err := h.service.ListVisible(c.Request.Context(), time.Now(), limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
