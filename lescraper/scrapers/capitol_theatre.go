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

const capitolTheatreURL = "https://www.thecapitoltheatre.com/events/venue/the-capitol-theatre/all"

// CapitolTheatre scraper handles The Capitol Theatre
type CapitolTheatre struct{}

func init() {
	// Register Capitol Theatre with the registry
	scraper := &CapitolTheatre{}
	core.RegisterVenue("The Capitol Theatre", scraper)
}

// GetVenues returns the venues managed by this scraper
func (s *CapitolTheatre) GetVenues() []string {
	return []string{"The Capitol Theatre"}
}

// GetName returns the scraper name
func (s *CapitolTheatre) GetName() string {
	return "Capitol Theatre"
}

// ScrapeShows implements the Scraper interface for The Capitol Theatre
func (s *CapitolTheatre) ScrapeShows() ([]core.Show, error) {
	// Fetch the page
	resp, err := http.Get(capitolTheatreURL)
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

	// Parse events - structure:
	// <div class="date" aria-label="October 16 2025">
	// ...
	// <h3 class="title">
	//   <a href="...">Artist Name</a>
	// </h3>

	// Match date aria-label and title in the same event block
	eventBlockRe := regexp.MustCompile(`(?s)<div class="eventItem[^>]*>.*?aria-label="([^"]+)".*?<h3 class="title[^>]*>.*?<a href="([^"]+)"[^>]*>([^<]+)</a>`)
	matches := eventBlockRe.FindAllStringSubmatch(htmlContent, -1)

	var shows []core.Show
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		dateStr := strings.TrimSpace(match[1])
		eventURL := match[2]
		title := strings.TrimSpace(match[3])

		// Filter out non-music events
		if !isMusicEvent(title) {
			continue
		}

		// Parse date (format: "October 16 2025")
		date, err := parseCapitolTheatreDate(dateStr)
		if err != nil {
			fmt.Printf("    Warning: could not parse date '%s' for %s\n", dateStr, title)
			continue
		}

		shows = append(shows, core.Show{
			Title:    title,
			Venue:    "The Capitol Theatre",
			Date:     date,
			Price:    "", // Price not available on calendar page
			URL:      eventURL,
			VenueURL: capitolTheatreURL,
		})
	}

	return shows, nil
}

// isMusicEvent filters out non-music events like closures, private events, and comedy shows
func isMusicEvent(title string) bool {
	titleLower := strings.ToLower(title)

	// Skip venue closures and private events
	skipKeywords := []string{
		"closed",
		"private event",
		"box office closed",
		"holiday",
	}

	for _, keyword := range skipKeywords {
		if strings.Contains(titleLower, keyword) {
			return false
		}
	}

	// Filter out comedy/talk shows
	// These usually have phrases that indicate non-music content
	comedyKeywords := []string{
		"an evening with", // Often used for talks/comedy
		"comedy",
		"comedian",
		"stand-up",
		"podcast",
	}

	// Check if it's likely a talk/comedy show
	// Look for specific patterns like "An Evening With [Person] of [TV Show]"
	if strings.Contains(titleLower, "an evening with") &&
	   (strings.Contains(titleLower, "\"") || strings.Contains(titleLower, "of")) {
		return false
	}

	for _, keyword := range comedyKeywords {
		if strings.Contains(titleLower, keyword) {
			return false
		}
	}

	return true
}

// parseCapitolTheatreDate parses dates in "October 16 2025" format
func parseCapitolTheatreDate(dateStr string) (time.Time, error) {
	// Format: "October 16 2025"
	date, err := time.Parse("January 2 2006", dateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date format: %s", dateStr)
	}

	// Set time to local timezone at midnight for proper comparison
	now := time.Now()
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, now.Location())

	return date, nil
}
