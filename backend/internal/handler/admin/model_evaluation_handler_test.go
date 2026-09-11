package admin

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type evaluationAdminStub struct {
	ModelEvaluationAdminService
	updated *service.ModelEvaluationConfig
}

func (s *evaluationAdminStub) UpdateConfig(_ context.Context, cfg service.ModelEvaluationConfig) (*service.ModelEvaluationConfig, error) {
	s.updated = &cfg
	return &cfg, nil
}
func TestModelEvaluationConfigRequiresExplicitBoolean(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{`{}`, `null`, `{"enabled":null}`, `{"enabled":"true"}`, `{"enabled":true,"api_key":"fake-sensitive-value"}`, `{"enabled":true} {}`} {
		t.Run(body, func(t *testing.T) {
			s := &evaluationAdminStub{}
			h := &ModelEvaluationHandler{service: s}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("PUT", "/config", strings.NewReader(body))
			h.UpdateConfig(c)
			require.Equal(t, 400, w.Code)
			require.Nil(t, s.updated)
			require.NotContains(t, w.Body.String(), "fake-sensitive-value")
		})
	}
	for _, body := range []string{`{"enabled":true}`, `{"enabled":false}`} {
		s := &evaluationAdminStub{}
		h := &ModelEvaluationHandler{service: s}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/config", strings.NewReader(body))
		h.UpdateConfig(c)
		require.Equal(t, 200, w.Code)
		require.NotNil(t, s.updated)
	}
}
func TestModelEvaluationBodyAndPaginationLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/tasks", strings.NewReader(`{"api_key":"`+strings.Repeat("x", 20000)+`"}`))
	var input service.ModelEvaluationTaskInput
	require.False(t, BindModelEvaluationJSON(c, &input))
	require.Equal(t, 400, w.Code)
	for _, query := range []string{"page=-1", "page_size=101", "group_id=-1", "task_id=invalid", "page=100001"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/results?"+query, nil)
		_, ok := ParseModelEvaluationList(c)
		require.False(t, ok)
		require.Equal(t, 400, w.Code)
	}
}
