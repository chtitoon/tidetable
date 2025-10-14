package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// VenueConfig represents the simple venue list
type VenueConfig struct {
	Venues []string `yaml:"venues"`
}

// LoadVenues loads venue names from a YAML file
func LoadVenues(filepath string) ([]string, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("reading venues file: %w", err)
	}

	var config VenueConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing venues YAML: %w", err)
	}

	if len(config.Venues) == 0 {
		return nil, fmt.Errorf("no venues found in configuration")
	}

	return config.Venues, nil
}
