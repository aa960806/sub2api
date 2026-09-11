package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelEvaluationTestAndPublicationRoutesStayAdminOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := &handler.Handlers{Admin: &handler.AdminHandlers{ModelEvaluation: admin.NewModelEvaluationHandler(nil, nil)}, ModelEvaluation: handler.NewModelEvaluationUserHandler(nil, nil)}
	adminGroup := router.Group("/api/v1/admin", func(c *gin.Context) { c.AbortWithStatus(http.StatusForbidden) })
	registerModelEvaluationAdminRoutes(adminGroup, h, nil)
	registerModelEvaluationUserRoutes(router.Group("/api/v1"), h, nil)
	for _, operation := range []struct{ method, path string }{{http.MethodPost, "/tasks/1/test"}, {http.MethodPut, "/tasks/1/publication"}} {
		for _, prefix := range []string{"/api/v1/admin/model-evaluations", "/api/v1/model-evaluations"} {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(operation.method, prefix+operation.path, nil))
			if prefix == "/api/v1/admin/model-evaluations" {
				require.Equal(t, http.StatusForbidden, w.Code, "admin parent gate must run before test/publication")
			} else {
				require.Equal(t, http.StatusNotFound, w.Code, "user router must never mount these actions")
			}
		}
	}
}
