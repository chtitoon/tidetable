package core

import "time"

// Show represents a concert/show at a venue
type Show struct {
	Title    string    // Full show title (may contain multiple artists)
	Venue    string    // Specific venue name (e.g., "Webster Hall")
	Date     time.Time
	Price    string
	URL      string
	VenueURL string
}

// NormalizedShow represents a show with cleaned and extracted artist information
type NormalizedShow struct {
	Artists  []string // Multiple artists extracted from the show title
	Venue    string
	Date     time.Time
	Price    string
	URL      string
	VenueURL string
}

// Scraper is the interface that all venue scrapers must implement
// A scraper typically represents a holding company (Bowery Presents, Mercury East, etc.)
// that manages multiple venues with a unified calendar system
type Scraper interface {
	// GetVenues returns all venue names this scraper can provide
	GetVenues() []string

	// ScrapeShows scrapes all shows from all venues managed by this scraper
	// Results should include the specific venue name in each Show.Venue field
	ScrapeShows() ([]Show, error)

	// GetName returns a human-readable name for this scraper
	GetName() string
}
