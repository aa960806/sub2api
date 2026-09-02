package admin

import (
	"io"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// SubNexusLeaderboardHandler manages leaderboard policy and the explicitly
// requested admin settlement/history endpoints. All financial work remains
// behind the service transaction and independent rollout gate.
type SubNexusLeaderboardHandler struct {
	service *service.LeaderboardService
}

func NewSubNexusLeaderboardHandler(s *service.LeaderboardService) *SubNexusLeaderboardHandler {
	return &SubNexusLeaderboardHandler{service: s}
}

func (h *SubNexusLeaderboardHandler) GetConfig(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_LEADERBOARD_UNAVAILABLE", "leaderboard service is unavailable"))
		return
	}
	response.Success(c, h.service.Config(c.Request.Context()))
}

func (h *SubNexusLeaderboardHandler) UpdateConfig(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_LEADERBOARD_UNAVAILABLE", "leaderboard service is unavailable"))
		return
	}
	var req service.LeaderboardConfig
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

type leaderboardRewardRequest struct {
	Window string `json:"window" form:"window"`
	Period string `json:"period" form:"period"`
}

// GrantRewards settles an explicit period when supplied. Without a period it
// settles the previous complete week/month, avoiding accidental current-period
// payouts from an admin click.
func (h *SubNexusLeaderboardHandler) GrantRewards(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_LEADERBOARD_UNAVAILABLE", "leaderboard service is unavailable"))
		return
	}
	var req leaderboardRewardRequest
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	window := strings.TrimSpace(req.Window)
	if window == "" {
		window = c.DefaultQuery("window", service.LeaderboardWindowWeek)
	}
	var (
		granted int
		total   float64
		err     error
	)
	if strings.TrimSpace(req.Period) != "" || strings.TrimSpace(c.Query("period")) != "" {
		period := strings.TrimSpace(req.Period)
		if period == "" {
			period = strings.TrimSpace(c.Query("period"))
		}
		granted, total, err = h.service.GrantLeaderboardRewardsForPeriod(c.Request.Context(), window, period)
	} else {
		granted, total, err = h.service.SettleCompletedLeaderboardPeriod(c.Request.Context(), window, timezone.Now())
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"window": window, "granted": granted, "total": total})
}

// ListRewardHistory returns paginated leaderboard grants with masked emails.
func (h *SubNexusLeaderboardHandler) ListRewardHistory(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_LEADERBOARD_UNAVAILABLE", "leaderboard service is unavailable"))
		return
	}
	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := parsePositiveInt(c.Query("page_size"), 20)
	if raw := c.Query("pageSize"); raw != "" {
		pageSize = parsePositiveInt(raw, pageSize)
	}
	history, err := h.service.ListRewardHistory(c.Request.Context(), page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, history)
}

func parsePositiveInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
