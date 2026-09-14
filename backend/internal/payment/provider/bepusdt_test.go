package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBepSignMatchesDocumentExampleShape(t *testing.T) {
	values := map[string]any{"order_id": "20220201030210321", "amount": 42, "notify_url": "http://example.com/notify", "redirect_url": "http://example.com/redirect"}
	require.Equal(t, "1cd4b52df5587cfb1968b0c0c6e156cd", bepSign(values, "epusdt_password_xasddawqe"))
}

func TestNewBepusdtValidation(t *testing.T) {
	_, err := NewBepusdt("x", map[string]string{"apiBase": "https://pay.example.com", "token": "t", "fiat": "USD"})
	require.Error(t, err)
	_, err = NewBepusdt("x", map[string]string{"apiBase": "https://pay.example.com", "token": "t", "timeout": "60"})
	require.Error(t, err)
}
