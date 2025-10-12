package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	TidalClientID     string
	TidalClientSecret string
	TidalAccessToken  string
	TidalRefreshToken string
	PlaylistNameBase  string
	VenuesFile        string
	DaysAhead         int
}

func Load() (*Config, error) {
	cfg := &Config{
		TidalClientID:     os.Getenv("TIDAL_CLIENT_ID"),
		TidalClientSecret: os.Getenv("TIDAL_CLIENT_SECRET"),
		TidalAccessToken:  os.Getenv("TIDAL_ACCESS_TOKEN"),
		TidalRefreshToken: os.Getenv("TIDAL_REFRESH_TOKEN"),
		PlaylistNameBase:  os.Getenv("PLAYLIST_NAME_BASE"),
		VenuesFile:        os.Getenv("VENUES_FILE"),
	}

	// Parse DAYS_AHEAD
	daysAheadStr := os.Getenv("DAYS_AHEAD")
	if daysAheadStr != "" {
		days, err := strconv.Atoi(daysAheadStr)
		if err != nil {
			return nil, fmt.Errorf("DAYS_AHEAD must be a number: %w", err)
		}
		cfg.DaysAhead = days
	}

	// Set defaults
	if cfg.PlaylistNameBase == "" {
		cfg.PlaylistNameBase = "Shows"
	}
	if cfg.VenuesFile == "" {
		cfg.VenuesFile = "venues.yaml"
	}
	if cfg.DaysAhead == 0 {
		cfg.DaysAhead = 7 // Default to 1 week
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
