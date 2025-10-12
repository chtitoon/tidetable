package venues

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// MercuryEastScraper handles Mercury East Presents venues (Mercury Lounge, Bowery Ballroom, etc.)
type MercuryEastScraper struct{}

func (s *MercuryEastScraper) ScrapeShows(venue Venue) ([]Show, error) {
	// Fetch the page
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

	var shows []Show

	// Parse event entries added via EventData.events.push()
	// Format: EventData.events.push({"value" : "Artist Name 10/12", ...});
	eventRe := regexp.MustCompile(`EventData\.events\.push\(\{[^}]*"value"\s*:\s*"([^"]+)"[^}]*"url"\s*:\s*"([^"]+)"`)
	eventMatches := eventRe.FindAllSubmatch(body, -1)

	currentYear := time.Now().Year()

	for _, match := range eventMatches {
		if len(match) < 3 {
			continue
		}

		value := string(match[1])
		url := string(match[2])

		// Skip sold out shows (marked with *)
		if strings.Contains(value, "*SOLD OUT*") || strings.Contains(value, "*POSTPONED*") {
			continue
		}

		// Parse "Artist Name 10/12" format
		parts := strings.Split(value, " ")
		if len(parts) < 2 {
			continue
		}

		datePart := parts[len(parts)-1]
		artistParts := parts[:len(parts)-1]
		artist := strings.TrimSpace(strings.Join(artistParts, " "))

		// Parse date (format: 10/12 or 1/5)
		date, err := parseMercuryEastDate(datePart, currentYear)
		if err != nil {
			fmt.Printf("    Warning: could not parse date '%s' for %s\n", datePart, artist)
			continue
		}

		shows = append(shows, Show{
			Artist:   artist,
			Venue:    venue.Name,
			Date:     date,
			Price:    "", // Price not available on calendar, would need to scrape individual event pages
			URL:      url,
			VenueURL: venue.URL,
		})
	}

	return shows, nil
}

// parseMercuryEastDate parses dates in "10/12" format (month/day)
func parseMercuryEastDate(dateStr string, year int) (time.Time, error) {
	parts := strings.Split(dateStr, "/")
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid date format: %s", dateStr)
	}

	// Try parsing as MM/DD
	dateWithYear := fmt.Sprintf("%s/%d", dateStr, year)
	t, err := time.Parse("1/2/2006", dateWithYear)
	if err != nil {
		return time.Time{}, err
	}

	// Convert to local timezone to properly compare with "today"
	now := time.Now()
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())

	// Compare dates without time component
	// If the date (midnight) is before today (midnight), assume it's next year
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if t.Before(today) {
		t = t.AddDate(1, 0, 0)
	}

	return t, nil
}

// BoweryPresentsScraper is an alias for MercuryEastScraper
type BoweryPresentsScraper = MercuryEastScraper

// GenericScraper is a fallback scraper that tries common patterns
type GenericScraper struct{}

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
