package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadSeedanceMediaDefaultsToDisabled(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.Gateway.Media.Enabled)
	require.Equal(t, int64(12*1024*1024), cfg.Gateway.Media.MaxImageBytes)
	require.Equal(t, int64(70*1024*1024), cfg.Gateway.Media.MaxImagesTotalBytes)
}

func TestLoadSeedanceMediaEnvironment(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_MEDIA_ENABLED", "true")
	t.Setenv("GATEWAY_MEDIA_MAX_IMAGE_BYTES", "1234")
	t.Setenv("GATEWAY_MEDIA_MAX_IMAGES_TOTAL_BYTES", "5678")
	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.Gateway.Media.Enabled)
	require.Equal(t, int64(1234), cfg.Gateway.Media.MaxImageBytes)
	require.Equal(t, int64(5678), cfg.Gateway.Media.MaxImagesTotalBytes)
}
