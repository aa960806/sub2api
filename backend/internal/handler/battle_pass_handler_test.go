package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBattlePassHandlerGetCurrentDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewBattlePassService(nil, &battlePassHandlerSettingStub{
		values: map[string]string{service.SettingKeyBattlePassEnabled: "false"},
	})
	h := NewBattlePassHandler(svc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/battle-pass/current", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})

	h.GetCurrent(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "BATTLE_PASS_DISABLED", body["reason"])
}

type battlePassHandlerSettingStub struct {
	values map[string]string
}

func (s *battlePassHandlerSettingStub) GetValue(_ context.Context, key string) (string, error) {
	if s == nil || s.values == nil {
		return "", nil
	}
	return s.values[key], nil
}

func (s *battlePassHandlerSettingStub) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}
