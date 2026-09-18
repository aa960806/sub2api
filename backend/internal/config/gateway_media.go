package config

// GatewayMediaConfig only applies to independent /v1/media and /media routes.
type GatewayMediaConfig struct {
	Enabled             bool  `mapstructure:"enabled"`
	MaxImageBytes       int64 `mapstructure:"max_image_bytes"`
	MaxImagesTotalBytes int64 `mapstructure:"max_images_total_bytes"`
}
