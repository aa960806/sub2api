package admin

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// SubNexusInviteActivitiesHandler manages the single atomic policy used by
// invite lottery, recharge wheel and invite milestone.  It intentionally
// remains available while the runtime switch is off so operators can prepare
// and review a policy before enabling any user-facing activity.
type SubNexusInviteActivitiesHandler struct {
	service *service.InviteActivitiesService
}

func NewSubNexusInviteActivitiesHandler(s *service.InviteActivitiesService) *SubNexusInviteActivitiesHandler {
	return &SubNexusInviteActivitiesHandler{service: s}
}

func (h *SubNexusInviteActivitiesHandler) GetConfig(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_INVITE_ACTIVITIES_UNAVAILABLE", "invite activity service is unavailable"))
		return
	}
	response.Success(c, h.service.Config(c.Request.Context()))
}

func (h *SubNexusInviteActivitiesHandler) UpdateConfig(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_INVITE_ACTIVITIES_UNAVAILABLE", "invite activity service is unavailable"))
		return
	}
	var req service.InviteActivitiesConfig
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
