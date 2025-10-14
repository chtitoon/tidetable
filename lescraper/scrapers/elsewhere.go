package scrapers

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/chtitoon/tidetable/lescraper/core"
)

const elsewhereURL = "https://www.elsewhere.club/events"

// Elsewhere scraper handles Elsewhere Brooklyn
type Elsewhere struct{}

func init() {
	// Register Elsewhere with the registry
	scraper := &Elsewhere{}
	core.RegisterVenue("Elsewhere", scraper)
}

// GetVenues returns the venues managed by this scraper
func (s *Elsewhere) GetVenues() []string {
	return []string{"Elsewhere"}
}

// GetName returns the scraper name
func (s *Elsewhere) GetName() string {
	return "Elsewhere"
}

// ScrapeShows implements the Scraper interface for Elsewhere
func (s *Elsewhere) ScrapeShows() ([]core.Show, error) {
	// Fetch the page
	resp, err := http.Get(elsewhereURL)
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

	htmlContent := string(body)

	// Parse events - structure uses schema.org markup:
	// <time ... dateTime="2025-10-15T23:00:00+00:00" itemProp="startDate">Wed, Oct 15</time>
	// <h2 ...><a ... itemProp="url" href="/events/123"><span itemProp="name">Artist Name</span></a>...

	// Match event blocks with dateTime and event name
	eventBlockRe := regexp.MustCompile(`(?s)dateTime="([^"]+)"[^>]*itemProp="startDate".*?<span itemProp="name">([^<]+)</span>.*?href="(/events/[^"]+)"`)
	matches := eventBlockRe.FindAllStringSubmatch(htmlContent, -1)

	var shows []core.Show
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		dateTimeStr := strings.TrimSpace(match[1])
		title := html.UnescapeString(strings.TrimSpace(match[2]))
		eventPath := match[3]

		// Skip "Elsewhere Presents" events (they're at other venues)
		if strings.Contains(title, "Elsewhere Presents:") {
			continue
		}

		// Parse date (format: "2025-10-15T23:00:00+00:00")
		date, err := parseElsewherDate(dateTimeStr)
		if err != nil {
			fmt.Printf("    Warning: could not parse date '%s' for %s\n", dateTimeStr, title)
			continue
		}

		eventURL := "https://www.elsewhere.club" + eventPath

		shows = append(shows, core.Show{
			Title:    title,
			Venue:    "Elsewhere",
			Date:     date,
			Price:    "", // Price not available on calendar page
			URL:      eventURL,
			VenueURL: elsewhereURL,
		})
	}

	return shows, nil
}

// parseElsewherDate parses dates in ISO 8601 format "2025-10-15T23:00:00+00:00"
func parseElsewherDate(dateStr string) (time.Time, error) {
	// Parse ISO 8601 format
	date, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date format: %s", dateStr)
	}

	// Convert to local timezone at midnight for proper comparison
	now := time.Now()
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, now.Location())

	return date, nil
}
