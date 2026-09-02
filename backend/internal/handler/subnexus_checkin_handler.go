package handler

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type SubNexusCheckInHandler struct{ service *service.CheckInService }

func NewSubNexusCheckInHandler(s *service.CheckInService) *SubNexusCheckInHandler {
	return &SubNexusCheckInHandler{service: s}
}
func (h *SubNexusCheckInHandler) Status(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_CHECKIN_UNAVAILABLE", "check-in service is unavailable"))
		return
	}
	v, err := h.service.Status(c.Request.Context(), subject.UserID, timezone.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, v)
}
func (h *SubNexusCheckInHandler) Claim(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_CHECKIN_UNAVAILABLE", "check-in service is unavailable"))
		return
	}
	v, err := h.service.Claim(c.Request.Context(), subject.UserID, ip.GetTrustedClientIP(c), timezone.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, v)
}
