package admin

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type SubNexusCheckInHandler struct{ service *service.CheckInService }

func NewSubNexusCheckInHandler(s *service.CheckInService) *SubNexusCheckInHandler {
	return &SubNexusCheckInHandler{service: s}
}
func (h *SubNexusCheckInHandler) GetConfig(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_CHECKIN_UNAVAILABLE", "check-in service is unavailable"))
		return
	}
	response.Success(c, h.service.Config(c.Request.Context()))
}
func (h *SubNexusCheckInHandler) UpdateConfig(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_CHECKIN_UNAVAILABLE", "check-in service is unavailable"))
		return
	}
	var req service.CheckInConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.service.UpdateConfig(c.Request.Context(), req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, cfg)
}
