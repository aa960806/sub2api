package handler

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

func (h *BattlePassHandler) GetCurrent(c *gin.Context) {
	userID, ok := battlePassUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if !h.requireService(c) {
		return
	}
	if err := h.battlePassService.RequireUserAccess(c.Request.Context(), time.Now()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	current, err := h.battlePassService.GetCurrentForUser(c.Request.Context(), userID, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, current)
}

func (h *BattlePassHandler) GetTasks(c *gin.Context) {
	userID, ok := battlePassUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if !h.requireService(c) {
		return
	}
	items, err := h.battlePassService.GetTasksForUser(c.Request.Context(), userID, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, items)
}

func (h *BattlePassHandler) GetRewards(c *gin.Context) {
	userID, ok := battlePassUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if !h.requireService(c) {
		return
	}
	items, err := h.battlePassService.GetRewardsForUser(c.Request.Context(), userID, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, items)
}

func (h *BattlePassHandler) ClaimReward(c *gin.Context) {
	userID, ok := battlePassUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if !h.requireService(c) {
		return
	}
	rewardID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || rewardID <= 0 {
		response.BadRequest(c, "Invalid reward id")
		return
	}
	result, err := h.battlePassService.ClaimRewardForUser(c.Request.Context(), userID, rewardID, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, result)
}

func (h *BattlePassHandler) ClaimAllRewards(c *gin.Context) {
	userID, ok := battlePassUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if !h.requireService(c) {
		return
	}
	result, err := h.battlePassService.ClaimAllRewardsForUser(c.Request.Context(), userID, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, result)
}

func (h *BattlePassHandler) GetHistory(c *gin.Context) {
	userID, ok := battlePassUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if !h.requireService(c) {
		return
	}
	now := time.Now()
	var items *service.BattlePassHistory
	var err error
	if rawSeasonID := c.Query("season_id"); rawSeasonID != "" {
		seasonID, parseErr := strconv.ParseInt(rawSeasonID, 10, 64)
		if parseErr != nil || seasonID <= 0 {
			response.BadRequest(c, "Invalid season_id")
			return
		}
		items, err = h.battlePassService.GetHistoryForUserSeason(c.Request.Context(), userID, seasonID, now)
	} else {
		items, err = h.battlePassService.GetHistoryForUser(c.Request.Context(), userID, now)
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, items)
}

func (h *BattlePassHandler) Purchase(c *gin.Context) {
	userID, ok := battlePassUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if !h.requireService(c) {
		return
	}
	var request struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			response.BadRequest(c, "Invalid request: "+err.Error())
			return
		}
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = c.GetHeader("Idempotency-Key")
	}
	purchase, err := h.battlePassService.PurchasePremium(c.Request.Context(), userID, request.IdempotencyKey, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, purchase)
}

func (h *BattlePassHandler) ListCosmetics(c *gin.Context) {
	userID, ok := battlePassUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if !h.requireService(c) {
		return
	}
	items, err := h.battlePassService.ListCosmeticsForUser(c.Request.Context(), userID, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, items)
}

func (h *BattlePassHandler) EquipCosmetic(c *gin.Context) {
	userID, ok := battlePassUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if !h.requireService(c) {
		return
	}
	var request struct {
		CosmeticID int64 `json:"cosmetic_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.CosmeticID <= 0 {
		response.BadRequest(c, "Invalid cosmetic_id")
		return
	}
	if err := h.battlePassService.EquipCosmetic(c.Request.Context(), userID, request.CosmeticID, time.Now()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"equipped": true})
}

func battlePassUserID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	return subject.UserID, ok && subject.UserID > 0
}

func (h *BattlePassHandler) requireService(c *gin.Context) bool {
	if h == nil || h.battlePassService == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("BATTLE_PASS_UNAVAILABLE", "battle pass service is unavailable"))
		return false
	}
	return true
}
