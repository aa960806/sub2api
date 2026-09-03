//go:build unit

package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSubNexusSensitiveAdminRoutesRequireStepUp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		StudentRechargeBenefit: adminhandler.NewStudentRechargeBenefitHandler(nil),
		Invoice:                adminhandler.NewInvoiceHandler(nil),
		SubNexusCheckIn:        adminhandler.NewSubNexusCheckInHandler(nil),
		SubNexusLeaderboard:    adminhandler.NewSubNexusLeaderboardHandler(nil),
	}}
	var calls int
	stepUp := middleware.StepUpAuthMiddleware(func(c *gin.Context) {
		calls++
		c.AbortWithStatus(http.StatusPreconditionRequired)
	})

	router := gin.New()
	admin := router.Group("/admin")
	registerStudentRechargeBenefitRoutes(admin, handlers, stepUp)
	registerInvoiceRoutes(admin, handlers, nil, stepUp)
	registerSubNexusCheckInRoutes(admin, handlers, stepUp)
	registerSubNexusLeaderboardRoutes(admin, handlers, stepUp)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/admin/activity/student-recharge/config"},
		{http.MethodPost, "/admin/activity/student-recharge/users/7/grant"},
		{http.MethodPost, "/admin/activity/student-recharge/users/7/revoke"},
		{http.MethodPut, "/admin/invoices/config"},
		{http.MethodPost, "/admin/invoices/7/accept"},
		{http.MethodPost, "/admin/invoices/7/release"},
		{http.MethodPost, "/admin/invoices/7/reject"},
		{http.MethodPost, "/admin/invoices/7/issue"},
		{http.MethodPost, "/admin/invoices/7/replace-file"},
		{http.MethodPost, "/admin/invoices/7/void"},
		{http.MethodPost, "/admin/invoices/7/resend-email"},
		{http.MethodPut, "/admin/checkin/config"},
		{http.MethodPut, "/admin/leaderboard/config"},
		{http.MethodPost, "/admin/leaderboard/rewards"},
	}
	for _, route := range routes {
		req := httptest.NewRequest(route.method, route.path, nil)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		require.Equalf(t, http.StatusPreconditionRequired, res.Code, "route %s", route.path)
	}
	require.Equal(t, len(routes), calls)
}
