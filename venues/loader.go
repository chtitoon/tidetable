package venues

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadVenues loads venue configuration from a YAML file
func LoadVenues(filepath string) ([]Venue, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("reading venues file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing venues YAML: %w", err)
	}

	if len(config.Venues) == 0 {
		return nil, fmt.Errorf("no venues found in configuration")
	}

	return config.Venues, nil
}
