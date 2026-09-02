package handler

import (
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// SubNexusLeaderboardHandler exposes read-only usage and invite boards. The
// service performs the independent rollout check before touching the database.
type SubNexusLeaderboardHandler struct {
	service *service.LeaderboardService
}

func NewSubNexusLeaderboardHandler(s *service.LeaderboardService) *SubNexusLeaderboardHandler {
	return &SubNexusLeaderboardHandler{service: s}
}

func (h *SubNexusLeaderboardHandler) GetLeaderboard(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_LEADERBOARD_UNAVAILABLE", "leaderboard service is unavailable"))
		return
	}
	window := c.DefaultQuery("window", service.LeaderboardWindowWeek)
	limit := parseLeaderboardLimit(c.Query("limit"))
	board, err := h.service.GetLeaderboard(c.Request.Context(), window, limit, timezone.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, board)
}

func (h *SubNexusLeaderboardHandler) GetInviteLeaderboard(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_LEADERBOARD_UNAVAILABLE", "leaderboard service is unavailable"))
		return
	}
	window := c.DefaultQuery("window", service.LeaderboardWindowWeek)
	limit := parseLeaderboardLimit(c.Query("limit"))
	board, err := h.service.GetInviteLeaderboard(c.Request.Context(), window, limit, timezone.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, board)
}

func parseLeaderboardLimit(raw string) int {
	if raw == "" {
		return 20
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 20
	}
	return value
}
