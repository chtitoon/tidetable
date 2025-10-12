package config

import (
	"fmt"
	"os"
)

type Config struct {
	TidalClientID     string
	TidalClientSecret string
	TidalAccessToken  string
	TidalRefreshToken string
	PlaylistName      string
	VenuesFile        string
}

func Load() (*Config, error) {
	cfg := &Config{
		TidalClientID:     os.Getenv("TIDAL_CLIENT_ID"),
		TidalClientSecret: os.Getenv("TIDAL_CLIENT_SECRET"),
		TidalAccessToken:  os.Getenv("TIDAL_ACCESS_TOKEN"),
		TidalRefreshToken: os.Getenv("TIDAL_REFRESH_TOKEN"),
		PlaylistName:      os.Getenv("PLAYLIST_NAME"),
		VenuesFile:        os.Getenv("VENUES_FILE"),
	}

	// Set defaults
	if cfg.PlaylistName == "" {
		cfg.PlaylistName = "This Week's Shows"
	}
	if cfg.VenuesFile == "" {
		cfg.VenuesFile = "venues.yaml"
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
