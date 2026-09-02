package handler

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ActivityCenterHandler struct {
	service *service.ActivityCenterService
}

func NewActivityCenterHandler(activityCenterService *service.ActivityCenterService) *ActivityCenterHandler {
	return &ActivityCenterHandler{service: activityCenterService}
}

func (h *ActivityCenterHandler) List(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SUBNEXUS_ACTIVITY_CENTER_UNAVAILABLE", "activity center service is unavailable"))
		return
	}
	result, err := h.service.ListVisible(c.Request.Context(), time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
