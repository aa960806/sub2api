package admin

import (
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// StudentRechargeBenefitHandler manages the independent student identity and
// ordinary-recharge offer.  It remains available while the runtime switch is
// off so an operator can prepare and audit the configuration first.
type StudentRechargeBenefitHandler struct {
	studentService *service.StudentRechargeBenefitService
}

func NewStudentRechargeBenefitHandler(studentService *service.StudentRechargeBenefitService) *StudentRechargeBenefitHandler {
	return &StudentRechargeBenefitHandler{studentService: studentService}
}

func (h *StudentRechargeBenefitHandler) serviceOrError() (*service.StudentRechargeBenefitService, error) {
	if h == nil || h.studentService == nil {
		return nil, infraerrors.ServiceUnavailable("STUDENT_BENEFIT_UNAVAILABLE", "student benefit service is unavailable")
	}
	return h.studentService, nil
}

// GetConfig returns the independent rollout configuration.
func (h *StudentRechargeBenefitHandler) GetConfig(c *gin.Context) {
	svc, err := h.serviceOrError()
	if err != nil {
		// A missing optional provider must not expose or enable the feature.
		response.Success(c, service.DefaultStudentRechargeBenefitConfig())
		return
	}
	cfg, err := svc.GetStudentRechargeBenefitConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *StudentRechargeBenefitHandler) UpdateConfig(c *gin.Context) {
	svc, err := h.serviceOrError()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var req service.StudentRechargeBenefitConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := svc.UpdateStudentRechargeBenefitConfig(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *StudentRechargeBenefitHandler) ListAccounts(c *gin.Context) {
	svc, err := h.serviceOrError()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if parsed, parseErr := strconv.Atoi(raw); parseErr == nil && parsed > 0 {
			limit = parsed
		}
	}
	items, err := svc.ListStudentAccounts(c.Request.Context(), c.Query("keyword"), limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *StudentRechargeBenefitHandler) GrantAccount(c *gin.Context) {
	h.setAccountStatus(c, true)
}

func (h *StudentRechargeBenefitHandler) RevokeAccount(c *gin.Context) {
	h.setAccountStatus(c, false)
}

func (h *StudentRechargeBenefitHandler) setAccountStatus(c *gin.Context, isStudent bool) {
	svc, err := h.serviceOrError()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user id")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Administrator not authenticated")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if c.Request.ContentLength != 0 {
		if bindErr := c.ShouldBindJSON(&req); bindErr != nil {
			response.BadRequest(c, "Invalid request: "+bindErr.Error())
			return
		}
	}
	item, err := svc.SetStudentAccountStatus(c.Request.Context(), userID, subject.UserID, isStudent, req.Reason, ip.GetTrustedClientIP(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *StudentRechargeBenefitHandler) ListAuditLogs(c *gin.Context) {
	svc, err := h.serviceOrError()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	limit := 100
	if raw := c.Query("limit"); raw != "" {
		if parsed, parseErr := strconv.Atoi(raw); parseErr == nil && parsed > 0 {
			limit = parsed
		}
	}
	items, err := svc.ListStudentAccountAuditLogs(c.Request.Context(), limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}
