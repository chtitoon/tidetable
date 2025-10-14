package core

import (
	"fmt"
	"sort"
	"sync"
)

// venueRegistry maps venue names to their scrapers
var venueRegistry = make(map[string]Scraper)
var registryMu sync.RWMutex

// RegisterVenue registers a venue name with its corresponding scraper
// This is typically called during package initialization by scraper implementations
func RegisterVenue(venueName string, scraper Scraper) {
	registryMu.Lock()
	defer registryMu.Unlock()
	venueRegistry[venueName] = scraper
}

// RegisterVenues registers multiple venue names with the same scraper
// This is a convenience function for scrapers that handle multiple venues
func RegisterVenues(venueNames []string, scraper Scraper) {
	for _, name := range venueNames {
		RegisterVenue(name, scraper)
	}
}

// GetScraperForVenue returns the scraper registered for a venue name
func GetScraperForVenue(venueName string) (Scraper, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	scraper, ok := venueRegistry[venueName]
	if !ok {
		return nil, fmt.Errorf("no scraper registered for venue: %s", venueName)
	}
	return scraper, nil
}

// GetSupportedVenues returns a sorted list of all registered venue names
func GetSupportedVenues() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	venues := make([]string, 0, len(venueRegistry))
	for name := range venueRegistry {
		venues = append(venues, name)
	}
	sort.Strings(venues)
	return venues
}

// GroupVenuesByScraper groups venue names by their scrapers to avoid duplicate scraping
// Returns a map of scraper -> list of venue names
func GroupVenuesByScraper(venueNames []string) (map[Scraper][]string, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	result := make(map[Scraper][]string)
	var missingVenues []string

	for _, venueName := range venueNames {
		scraper, ok := venueRegistry[venueName]
		if !ok {
			missingVenues = append(missingVenues, venueName)
			continue
		}
		result[scraper] = append(result[scraper], venueName)
	}

	if len(missingVenues) > 0 {
		return nil, fmt.Errorf("unsupported venues: %v", missingVenues)
	}

	return result, nil
}
