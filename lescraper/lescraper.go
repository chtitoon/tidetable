// Package lescraper provides a unified interface for scraping concert venues
// Users specify venue names, and lescraper handles the rest
package lescraper

import (
	"fmt"
	"sort"
	"time"

	"github.com/chtitoon/tidetable/lescraper/core"
	// Import scrapers to trigger their init() functions which register venues
	_ "github.com/chtitoon/tidetable/lescraper/scrapers"
)

// Show is an alias for core.Show for convenience
type Show = core.Show

// NormalizedShow is an alias for core.NormalizedShow for convenience
type NormalizedShow = core.NormalizedShow

// ScrapeVenues scrapes shows from the specified venues within the date range
// Returns all shows sorted chronologically
func ScrapeVenues(venueNames []string, startDate, endDate time.Time) ([]Show, error) {
	if len(venueNames) == 0 {
		return nil, fmt.Errorf("no venues specified")
	}

	// Group venues by their scrapers to avoid duplicate scraping
	scraperGroups, err := core.GroupVenuesByScraper(venueNames)
	if err != nil {
		return nil, err
	}

	var allShows []Show

	// Scrape each group
	for scraper, requestedVenues := range scraperGroups {
		fmt.Printf("Scraping %s (%d venue(s))...\n", scraper.GetName(), len(requestedVenues))

		shows, err := scraper.ScrapeShows()
		if err != nil {
			return nil, fmt.Errorf("scraping failed: %w", err)
		}

		// Filter to only requested venues
		requestedVenueSet := make(map[string]bool)
		for _, v := range requestedVenues {
			requestedVenueSet[v] = true
		}

		for _, show := range shows {
			if requestedVenueSet[show.Venue] {
				allShows = append(allShows, show)
			}
		}
	}

	// Filter by date range
	var filteredShows []Show
	for _, show := range allShows {
		// Normalize dates to midnight for comparison
		showDate := time.Date(show.Date.Year(), show.Date.Month(), show.Date.Day(), 0, 0, 0, 0, show.Date.Location())
		start := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
		end := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, endDate.Location())

		if (showDate.Equal(start) || showDate.After(start)) && (showDate.Equal(end) || showDate.Before(end)) {
			filteredShows = append(filteredShows, show)
		}
	}

	// Sort by date
	sort.Slice(filteredShows, func(i, j int) bool {
		return filteredShows[i].Date.Before(filteredShows[j].Date)
	})

	return filteredShows, nil
}

// NormalizeShows processes raw shows and extracts individual artists
func NormalizeShows(shows []Show) []NormalizedShow {
	return core.NormalizeShows(shows)
}

// GetSupportedVenues returns a sorted list of all supported venue names
func GetSupportedVenues() []string {
	return core.GetSupportedVenues()
}

// RegisterVenue allows users to register custom venues with their own scrapers
// This enables extending lescraper with new venue support
func RegisterVenue(venueName string, scraper core.Scraper) {
	core.RegisterVenue(venueName, scraper)
}
