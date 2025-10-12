package venues

import (
	"fmt"
	"time"
)

// Scraper is the interface for venue-specific scrapers
type Scraper interface {
	// ScrapeShows fetches upcoming shows from the venue
	ScrapeShows(venue Venue) ([]Show, error)
}

// GetScraper returns the appropriate scraper for a venue type
func GetScraper(scraperType string) (Scraper, error) {
	switch scraperType {
	case "mercury_east":
		return &MercuryEastScraper{}, nil
	case "bowery_presents":
		return &BoweryPresentsScraper{}, nil
	case "generic":
		return &GenericScraper{}, nil
	default:
		return nil, fmt.Errorf("unknown scraper type: %s", scraperType)
	}
}

// ScrapeAllVenuesForWeek scrapes all configured venues and returns shows happening between start and end dates
func ScrapeAllVenuesForWeek(venues []Venue, startDate, endDate time.Time) ([]Show, error) {
	var allShows []Show

	for _, venue := range venues {
		fmt.Printf("Scraping %s...\n", venue.Name)

		scraper, err := GetScraper(venue.ScraperType)
		if err != nil {
			fmt.Printf("  Warning: %v, skipping\n", err)
			continue
		}

		shows, err := scraper.ScrapeShows(venue)
		if err != nil {
			fmt.Printf("  Error scraping %s: %v\n", venue.Name, err)
			continue
		}

		fmt.Printf("  Scraped %d total shows\n", len(shows))

		// Filter for shows within the date range (inclusive)
		matchingShows := 0
		for _, show := range shows {
			// Check if show is on or after startDate and on or before endDate
			showDate := time.Date(show.Date.Year(), show.Date.Month(), show.Date.Day(), 0, 0, 0, 0, show.Date.Location())
			start := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
			end := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 0, endDate.Location())

			if (showDate.Equal(start) || showDate.After(start)) && (showDate.Equal(end) || showDate.Before(end)) {
				allShows = append(allShows, show)
				matchingShows++
			}
		}

		fmt.Printf("  Found %d shows this week\n", matchingShows)
	}

	return allShows, nil
}
