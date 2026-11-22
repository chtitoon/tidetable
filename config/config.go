package config

import (
	"fmt"
	"os"
)

// Config holds the Tidal credentials used by the daily artist export.
// (The show export needs none; it only reads VENUES_FILE, see main.go.)
type Config struct {
	TidalClientID     string
	TidalClientSecret string
	TidalAccessToken  string
	TidalRefreshToken string
}

func Load() (*Config, error) {
	cfg := &Config{
		TidalClientID:     os.Getenv("TIDAL_CLIENT_ID"),
		TidalClientSecret: os.Getenv("TIDAL_CLIENT_SECRET"),
		TidalAccessToken:  os.Getenv("TIDAL_ACCESS_TOKEN"),
		TidalRefreshToken: os.Getenv("TIDAL_REFRESH_TOKEN"),
	}

	// Validate required fields
	if cfg.TidalClientID == "" {
		return nil, fmt.Errorf("TIDAL_CLIENT_ID is required")
	}
	if cfg.TidalClientSecret == "" {
		return nil, fmt.Errorf("TIDAL_CLIENT_SECRET is required")
	}
	if cfg.TidalAccessToken == "" {
		return nil, fmt.Errorf("TIDAL_ACCESS_TOKEN is required")
	}
	if cfg.TidalRefreshToken == "" {
		return nil, fmt.Errorf("TIDAL_REFRESH_TOKEN is required")
	}

	return cfg, nil
}
