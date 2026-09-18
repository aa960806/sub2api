//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSeedanceAvailableModelsHaveUsableDropdownLabels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newStubAdminService()
	svc.getAccountResult = &service.Account{ID: 1, Platform: service.PlatformSeedance, Type: service.AccountTypeAPIKey}
	h := &AccountHandler{adminService: svc}
	r := gin.New()
	r.GET("/accounts/:id/models", h.GetAvailableModels)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/accounts/1/models", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Data []openai.Model `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data, 4)
	for i, model := range response.Data {
		require.Equal(t, service.DefaultSeedanceModelIDs()[i], model.ID)
		require.Equal(t, model.ID, model.DisplayName)
		require.Equal(t, "model", model.Type)
		require.Equal(t, service.PlatformSeedance, model.OwnedBy)
	}
}
