package scrapers

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/chtitoon/tidetable/lescraper/core"
)

const boweryPresentsCalendarURL = "https://www.bowerypresents.com/new-york-metro/calendar/"

// BoweryPresents scraper handles all Bowery Presents venues
// Scrapes the unified New York Metro calendar which lists all venues together
type BoweryPresents struct{}

// Venues managed by Bowery Presents (updated as new venues are discovered)
var boweryPresentsVenues = []string{
	"Webster Hall",
	"Music Hall of Williamsburg",
	"Brooklyn Steel",
	"Terminal 5",
	"Racket NYC",
	"The Rooftop at Pier 17",
	// Add more as discovered from the calendar
}

func init() {
	// Register all Bowery Presents venues with the registry
	scraper := &BoweryPresents{}
	core.RegisterVenues(scraper.GetVenues(), scraper)
}

// GetVenues returns all venues managed by Bowery Presents
func (s *BoweryPresents) GetVenues() []string {
	return boweryPresentsVenues
}

// GetName returns the scraper name
func (s *BoweryPresents) GetName() string {
	return "Bowery Presents"
}

// ScrapeShows implements the Scraper interface for Bowery Presents venues
func (s *BoweryPresents) ScrapeShows() ([]core.Show, error) {
	// Fetch the page
	resp, err := http.Get(boweryPresentsCalendarURL)
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

	// Parse event blocks - each event has this HTML structure:
	// <h4 class="event-headliner event-text">Artist Name</h4>
	// ...
	// <p class="event-date event-text">Mon, October 14, 2025</p>
	// ...
	// <p class="event-venue event-text">Webster Hall</p>
	// ...
	// <a href="https://www.axs.com/events/123/artist-name-tickets">

	// Extract all event blocks using regex
	eventBlockRe := regexp.MustCompile(`(?s)class="event-headliner event-text">([^<]+)</h4>.*?class="event-date event-text">([^<]+)</p>.*?class="event-venue event-text">([^<]+)</p>.*?href="(https://www\.axs\.com/events/[^"]+)"`)
	matches := eventBlockRe.FindAllStringSubmatch(htmlContent, -1)

	var shows []core.Show
	for _, match := range matches {
		if len(match) < 5 {
			continue
		}

		title := strings.TrimSpace(match[1])
		dateStr := strings.TrimSpace(match[2])
		venueName := strings.TrimSpace(match[3])
		eventURL := match[4]

		// Parse date (format: "Mon, October 14, 2025")
		date, err := parseBoweryPresentsDate(dateStr)
		if err != nil {
			fmt.Printf("    Warning: could not parse date '%s' for %s\n", dateStr, title)
			continue
		}

		// Return all shows from the calendar
		// The actual venue name comes from the page (Webster Hall, Music Hall of Williamsburg, etc.)
		shows = append(shows, core.Show{
			Title:    title,
			Venue:    venueName,
			Date:     date,
			Price:    "", // Price not available on calendar page
			URL:      eventURL,
			VenueURL: boweryPresentsCalendarURL,
		})
	}

	return shows, nil
}

// parseBoweryPresentsDate parses dates in "Mon, October 14, 2025" format
func parseBoweryPresentsDate(dateStr string) (time.Time, error) {
	// Primary format: "Mon, October 14, 2025"
	date, err := time.Parse("Mon, January 2, 2006", dateStr)
	if err != nil {
		// Try alternate format without day name: "October 14, 2025"
		date, err = time.Parse("January 2, 2006", dateStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid date format: %s", dateStr)
		}
	}

	// Set time to local timezone at midnight for proper comparison
	now := time.Now()
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, now.Location())

	return date, nil
}
