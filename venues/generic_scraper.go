package venues

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// GenericScraper is a fallback scraper that tries common patterns
// This should be customized per venue as needed
type GenericScraper struct{}

// ScrapeShows implements the Scraper interface with basic generic logic
func (s *GenericScraper) ScrapeShows(venue Venue) ([]Show, error) {
	resp, err := http.Get(venue.URL)
	if err != nil {
		return nil, fmt.Errorf("fetching page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}

	// This is a very basic implementation
	// In practice, you'll need custom scrapers for each venue
	fmt.Printf("  Generic scraper not fully implemented for %s\n", venue.Name)
	fmt.Printf("  Please implement a custom scraper for this venue type\n")
	fmt.Printf("  Page size: %d bytes\n", len(body))

	return []Show{}, nil
}

// parseDate attempts to parse common date formats from venue websites
func parseDate(dateStr string) (time.Time, error) {
	// Clean up the date string
	dateStr = strings.TrimSpace(dateStr)
	dateStr = regexp.MustCompile(`\s+`).ReplaceAllString(dateStr, " ")

	// Try common formats
	formats := []string{
		"Monday, January 2, 2006",
		"Mon, Jan 2, 2006",
		"January 2, 2006",
		"Jan 2, 2006",
		"01/02/2006",
		"1/2/2006",
		"2006-01-02",
		"Mon Jan 2 2006",
		"Mon, Jan 02, 2006",
		time.RFC3339,
		time.RFC1123,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("could not parse date: %s", dateStr)
}
