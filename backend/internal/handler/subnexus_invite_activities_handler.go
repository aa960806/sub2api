package handler

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// SubNexusInviteActivitiesHandler exposes the three migrated invite/recharge
// reward activities.  The service owns the independent feature gates and
// returns a disabled status without touching activity tables when they are off.
type SubNexusInviteActivitiesHandler struct {
	service *service.InviteActivitiesService
}

func NewSubNexusInviteActivitiesHandler(s *service.InviteActivitiesService) *SubNexusInviteActivitiesHandler {
	return &SubNexusInviteActivitiesHandler{service: s}
}

func (h *SubNexusInviteActivitiesHandler) GetInviteLotteryStatus(c *gin.Context) {
	userID, ok := inviteActivityUserID(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_INVITE_ACTIVITIES_UNAVAILABLE", "invite activity service is unavailable"))
		return
	}
	status, err := h.service.GetInviteLotteryStatus(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func (h *SubNexusInviteActivitiesHandler) ClaimInviteLottery(c *gin.Context) {
	userID, ok := inviteActivityUserID(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_INVITE_ACTIVITIES_UNAVAILABLE", "invite activity service is unavailable"))
		return
	}
	status, err := h.service.ClaimInviteLottery(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func (h *SubNexusInviteActivitiesHandler) GetRechargeWheelStatus(c *gin.Context) {
	userID, ok := inviteActivityUserID(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_INVITE_ACTIVITIES_UNAVAILABLE", "invite activity service is unavailable"))
		return
	}
	status, err := h.service.GetRechargeWheelStatus(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func (h *SubNexusInviteActivitiesHandler) ClaimRechargeWheel(c *gin.Context) {
	userID, ok := inviteActivityUserID(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_INVITE_ACTIVITIES_UNAVAILABLE", "invite activity service is unavailable"))
		return
	}
	status, err := h.service.ClaimRechargeWheel(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func (h *SubNexusInviteActivitiesHandler) GetInviteMilestoneStatus(c *gin.Context) {
	userID, ok := inviteActivityUserID(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_INVITE_ACTIVITIES_UNAVAILABLE", "invite activity service is unavailable"))
		return
	}
	status, err := h.service.GetInviteMilestoneStatus(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func (h *SubNexusInviteActivitiesHandler) ClaimInviteMilestone(c *gin.Context) {
	userID, ok := inviteActivityUserID(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_INVITE_ACTIVITIES_UNAVAILABLE", "invite activity service is unavailable"))
		return
	}
	var req struct {
		Invites int `json:"invites"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Invites <= 0 {
		response.BadRequest(c, "invalid milestone")
		return
	}
	status, err := h.service.ClaimInviteMilestone(c.Request.Context(), userID, req.Invites)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func inviteActivityUserID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return 0, false
	}
	return subject.UserID, true
}
