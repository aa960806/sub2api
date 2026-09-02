package handler

import (
	"math"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// StudentRechargeBenefitHandler exposes the user-facing student recharge
// offer.  The service owns all eligibility and amount calculations; this
// handler only authenticates the caller and parses the display amount.
type StudentRechargeBenefitHandler struct {
	studentService *service.StudentRechargeBenefitService
}

func NewStudentRechargeBenefitHandler(studentService *service.StudentRechargeBenefitService) *StudentRechargeBenefitHandler {
	return &StudentRechargeBenefitHandler{studentService: studentService}
}

// GetStatus returns a fail-closed status snapshot for the current user.
// GET /api/v1/activity/student-recharge/status
func (h *StudentRechargeBenefitHandler) GetStatus(c *gin.Context) {
	userID, ok := studentBenefitUserID(c)
	if !ok {
		return
	}
	if h == nil || h.studentService == nil {
		response.Success(c, service.StudentRechargeBenefitStatus{})
		return
	}
	status, err := h.studentService.GetStudentRechargeBenefitStatus(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

// Quote returns the server-authoritative credited and bonus amounts.
// GET /api/v1/activity/student-recharge/quote?amount=...
func (h *StudentRechargeBenefitHandler) Quote(c *gin.Context) {
	userID, ok := studentBenefitUserID(c)
	if !ok {
		return
	}
	amount := 0.0
	if parsed, err := strconv.ParseFloat(c.Query("amount"), 64); err == nil && parsed > 0 && !math.IsNaN(parsed) && !math.IsInf(parsed, 0) {
		amount = parsed
	}
	if h == nil || h.studentService == nil {
		response.Success(c, &service.StudentRechargeBenefitQuote{RechargeAmount: amount})
		return
	}
	quote, err := h.studentService.QuoteStudentRechargeBenefit(c.Request.Context(), userID, amount)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, quote)
}

func studentBenefitUserID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return 0, false
	}
	return subject.UserID, true
}
