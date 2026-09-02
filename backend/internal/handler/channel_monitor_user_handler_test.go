package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newChannelMonitorUserHandlerTestContext(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, nil)
	return c, recorder
}

func TestChannelMonitorUserHandlerList_NilSettingsFailsClosed(t *testing.T) {
	c, recorder := newChannelMonitorUserHandlerTestContext(http.MethodGet, "/api/v1/channel-monitors")
	h := NewChannelMonitorUserHandler(nil, nil)

	h.List(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"code":0,"message":"success","data":{"items":[]}}`, recorder.Body.String())
}

func TestChannelMonitorUserHandlerGetStatus_NilSettingsFailsClosed(t *testing.T) {
	c, recorder := newChannelMonitorUserHandlerTestContext(http.MethodGet, "/api/v1/channel-monitors/123/status")
	c.Params = gin.Params{{Key: "id", Value: "123"}}
	h := NewChannelMonitorUserHandler(nil, nil)

	h.GetStatus(c)

	require.Equal(t, http.StatusNotFound, recorder.Code)
}
