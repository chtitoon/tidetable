package venues

import "time"

// Venue represents a music venue configuration
type Venue struct {
	Name        string `yaml:"name"`
	URL         string `yaml:"url"`
	Location    string `yaml:"location"`
	ScraperType string `yaml:"scraper_type"`
}

// Show represents a concert/show at a venue
type Show struct {
	Title     string    // Full show title (may contain multiple artists)
	Venue     string
	Date      time.Time
	Price     string
	URL       string
	VenueURL  string
}

// Config holds the venues configuration
type Config struct {
	Venues []Venue `yaml:"venues"`
}
