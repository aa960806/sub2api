package admin

import (
	"strconv"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type BattlePassHandler struct {
	battlePassService *service.BattlePassService
}

func NewBattlePassHandler(battlePassService *service.BattlePassService) *BattlePassHandler {
	return &BattlePassHandler{battlePassService: battlePassService}
}

func (h *BattlePassHandler) GetSettings(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	settings, err := h.battlePassService.GetSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *BattlePassHandler) UpdateSettings(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req service.BattlePassSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	settings, err := h.battlePassService.SetEnabled(c.Request.Context(), req.Enabled)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *BattlePassHandler) ListSeasons(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	items, err := h.battlePassService.ListSeasons(c.Request.Context(), time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *BattlePassHandler) CreateSeason(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req service.BattlePassSeasonDraft
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.battlePassService.CreateSeason(c.Request.Context(), req, battlePassAdminID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *BattlePassHandler) GetSeason(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := battlePassRouteID(c, "id")
	if !ok {
		return
	}
	item, err := h.battlePassService.GetSeason(c.Request.Context(), id, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *BattlePassHandler) UpdateSeason(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := battlePassRouteID(c, "id")
	if !ok {
		return
	}
	var req service.BattlePassSeasonDraft
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.battlePassService.UpdateSeason(c.Request.Context(), id, req, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *BattlePassHandler) ValidateSeason(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := battlePassRouteID(c, "id")
	if !ok {
		return
	}
	result, err := h.battlePassService.ValidateSeason(c.Request.Context(), id, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *BattlePassHandler) PublishSeason(c *gin.Context) {
	h.mutateSeason(c, func(id, _ int64) (*service.BattlePassSeasonDetail, error) {
		return h.battlePassService.PublishSeason(c.Request.Context(), id, time.Now())
	})
}

func (h *BattlePassHandler) PauseSeason(c *gin.Context) {
	h.mutateSeason(c, func(id, adminID int64) (*service.BattlePassSeasonDetail, error) {
		return h.battlePassService.PauseSeason(c.Request.Context(), id, adminID, "", time.Now())
	})
}

func (h *BattlePassHandler) ResumeSeason(c *gin.Context) {
	h.mutateSeason(c, func(id, _ int64) (*service.BattlePassSeasonDetail, error) {
		return h.battlePassService.ResumeSeason(c.Request.Context(), id, time.Now())
	})
}

func (h *BattlePassHandler) EndSeason(c *gin.Context) {
	h.mutateSeason(c, func(id, _ int64) (*service.BattlePassSeasonDetail, error) {
		return h.battlePassService.EndSeason(c.Request.Context(), id, time.Now())
	})
}

func (h *BattlePassHandler) ListSeasonUsers(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := battlePassRouteID(c, "id")
	if !ok {
		return
	}
	items, err := h.battlePassService.ListSeasonUsers(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *BattlePassHandler) ListSeasonGrants(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := battlePassRouteID(c, "id")
	if !ok {
		return
	}
	items, err := h.battlePassService.ListSeasonGrants(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *BattlePassHandler) ListSeasonPurchases(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := battlePassRouteID(c, "id")
	if !ok {
		return
	}
	items, err := h.battlePassService.ListSeasonPurchases(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *BattlePassHandler) RetryGrant(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := battlePassRouteID(c, "id")
	if !ok {
		return
	}
	if err := h.battlePassService.RetryRewardGrant(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"retried": true})
}

func (h *BattlePassHandler) GetTestState(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	seasonID, err := strconv.ParseInt(c.Query("season_id"), 10, 64)
	if err != nil || seasonID <= 0 {
		response.BadRequest(c, "invalid season id")
		return
	}
	userID := battlePassAdminID(c)
	if raw := c.Query("user_id"); raw != "" {
		userID, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			response.BadRequest(c, "invalid user id")
			return
		}
	}
	state, err := h.battlePassService.GetTestState(c.Request.Context(), seasonID, userID, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, state)
}

func (h *BattlePassHandler) ActivateSeasonForTest(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req service.BattlePassTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	season, err := h.battlePassService.ActivateSeasonForTest(c.Request.Context(), req.SeasonID, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, season)
}

func (h *BattlePassHandler) CompleteTasksForTest(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req service.BattlePassTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.UserID <= 0 {
		req.UserID = battlePassAdminID(c)
	}
	result, err := h.battlePassService.CompleteTasksForTest(c.Request.Context(), req, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *BattlePassHandler) mutateSeason(c *gin.Context, fn func(id, adminID int64) (*service.BattlePassSeasonDetail, error)) {
	if !h.requireService(c) {
		return
	}
	id, ok := battlePassRouteID(c, "id")
	if !ok {
		return
	}
	item, err := fn(id, battlePassAdminID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func battlePassRouteID(c *gin.Context, key string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(key), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid battle pass id")
		return 0, false
	}
	return id, true
}

func battlePassAdminID(c *gin.Context) int64 {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		return 0
	}
	return subject.UserID
}

func (h *BattlePassHandler) requireService(c *gin.Context) bool {
	if h == nil || h.battlePassService == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("BATTLE_PASS_UNAVAILABLE", "battle pass service is unavailable"))
		return false
	}
	return true
}
