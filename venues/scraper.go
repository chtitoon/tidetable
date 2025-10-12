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
	case "mercury_east", "bowery_presents":
		return &MercuryEastScraper{}, nil
	case "generic":
		return &GenericScraper{}, nil
	default:
		return nil, fmt.Errorf("unknown scraper type: %s", scraperType)
	}
}

// ScrapeAllVenues scrapes all configured venues and returns shows happening in the next week
func ScrapeAllVenues(venues []Venue) ([]Show, error) {
	var allShows []Show
	now := time.Now()
	oneWeekFromNow := now.AddDate(0, 0, 7)

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

		// Filter for shows in the next week (including today)
		weekShows := 0
		for _, show := range shows {
			// Use After(now.AddDate(0,0,-1)) to include shows today
			if show.Date.After(now.AddDate(0, 0, -1)) && show.Date.Before(oneWeekFromNow) {
				allShows = append(allShows, show)
				weekShows++
			}
		}

		fmt.Printf("  Found %d shows this week\n", weekShows)
	}

	return allShows, nil
}
