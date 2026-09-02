package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBattlePassRouteContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewBattlePassService(nil, &battlePassRouteSettings{
		values: map[string]string{service.SettingKeyBattlePassEnabled: "false"},
	})
	handlers := &handler.Handlers{
		BattlePass: handler.NewBattlePassHandler(svc),
		Admin: &handler.AdminHandlers{
			BattlePass: adminhandler.NewBattlePassHandler(svc),
		},
	}
	router := gin.New()
	pass := func(c *gin.Context) { c.Next() }
	RegisterUserRoutes(
		router.Group("/api/v1"),
		handlers,
		middleware.JWTAuthMiddleware(pass),
		middleware.AuditLogMiddleware(pass),
		nil,
		nil,
	)
	RegisterAdminRoutes(
		router.Group("/api/v1"),
		handlers,
		middleware.AdminAuthMiddleware(pass),
		middleware.AuditLogMiddleware(pass),
		middleware.StepUpAuthMiddleware(pass),
		nil,
		nil,
	)

	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range []string{
		"GET /api/v1/battle-pass/current",
		"GET /api/v1/battle-pass/current/tasks",
		"GET /api/v1/battle-pass/current/rewards",
		"POST /api/v1/battle-pass/current/rewards/claim-all",
		"POST /api/v1/battle-pass/current/rewards/:id/claim",
		"GET /api/v1/battle-pass/current/history",
		"POST /api/v1/battle-pass/current/purchase",
		"GET /api/v1/battle-pass/cosmetics",
		"PUT /api/v1/battle-pass/cosmetics/equipped",
		"GET /api/v1/admin/activity/battle-pass/settings",
		"PUT /api/v1/admin/activity/battle-pass/settings",
		"GET /api/v1/admin/activity/battle-pass/seasons",
		"POST /api/v1/admin/activity/battle-pass/seasons",
		"GET /api/v1/admin/activity/battle-pass/seasons/:id",
		"PUT /api/v1/admin/activity/battle-pass/seasons/:id",
		"POST /api/v1/admin/activity/battle-pass/seasons/:id/validate",
		"POST /api/v1/admin/activity/battle-pass/seasons/:id/publish",
		"POST /api/v1/admin/activity/battle-pass/seasons/:id/pause",
		"POST /api/v1/admin/activity/battle-pass/seasons/:id/resume",
		"POST /api/v1/admin/activity/battle-pass/seasons/:id/end",
		"GET /api/v1/admin/activity/battle-pass/seasons/:id/users",
		"GET /api/v1/admin/activity/battle-pass/seasons/:id/grants",
		"GET /api/v1/admin/activity/battle-pass/seasons/:id/purchases",
		"POST /api/v1/admin/activity/battle-pass/grants/:id/retry",
		"GET /api/v1/admin/activity/battle-pass/test/state",
		"POST /api/v1/admin/activity/battle-pass/test/activate",
		"POST /api/v1/admin/activity/battle-pass/test/complete",
	} {
		_, ok := routes[route]
		require.Truef(t, ok, "missing route %s", route)
	}
}

func TestBattlePassAdminMutationsRequireStepUp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewBattlePassService(nil, &battlePassRouteSettings{})
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		BattlePass: adminhandler.NewBattlePassHandler(svc),
	}}
	var stepUpCalls int
	router := gin.New()
	registerBattlePassRoutes(
		router.Group("/api/v1/admin"),
		handlers,
		middleware.StepUpAuthMiddleware(func(c *gin.Context) {
			stepUpCalls++
			c.AbortWithStatus(http.StatusPreconditionRequired)
		}),
	)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/activity/battle-pass/settings", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	require.Equal(t, http.StatusPreconditionRequired, res.Code)
	require.Equal(t, 1, stepUpCalls)
}

type battlePassRouteSettings struct {
	values map[string]string
}

func (s *battlePassRouteSettings) GetValue(_ context.Context, key string) (string, error) {
	if s == nil || s.values == nil {
		return "", nil
	}
	return s.values[key], nil
}

func (s *battlePassRouteSettings) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}
