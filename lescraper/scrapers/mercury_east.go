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

// MercuryEast scraper handles all Mercury East Presents venues
// Each venue has its own calendar URL with the same structure
type MercuryEast struct{}

// Venue info for Mercury East venues
type mercuryEastVenue struct {
	Name string
	URL  string
}

// Mercury East Presents venues with their calendar URLs
var mercuryEastVenues = []mercuryEastVenue{
	{Name: "Mercury Lounge", URL: "https://mercuryeastpresents.com/calendar-mr/"},
	{Name: "Bowery Ballroom", URL: "https://mercuryeastpresents.com/boweryballroom/"},
	// Add more Mercury East venues as discovered
}

func init() {
	// Register all Mercury East venues with the registry
	scraper := &MercuryEast{}
	core.RegisterVenues(scraper.GetVenues(), scraper)
}

// GetVenues returns all venues managed by Mercury East Presents
func (s *MercuryEast) GetVenues() []string {
	venues := make([]string, len(mercuryEastVenues))
	for i, v := range mercuryEastVenues {
		venues[i] = v.Name
	}
	return venues
}

// GetName returns the scraper name
func (s *MercuryEast) GetName() string {
	return "Mercury East"
}

// ScrapeShows implements the Scraper interface for Mercury East venues
func (s *MercuryEast) ScrapeShows() ([]core.Show, error) {
	var allShows []core.Show

	// Scrape each Mercury East venue
	for _, venue := range mercuryEastVenues {
		shows, err := s.scrapeVenue(venue)
		if err != nil {
			// Log error but continue with other venues
			fmt.Printf("Warning: failed to scrape %s: %v\n", venue.Name, err)
			continue
		}
		allShows = append(allShows, shows...)
	}

	return allShows, nil
}

// scrapeVenue scrapes a single Mercury East venue
func (s *MercuryEast) scrapeVenue(venue mercuryEastVenue) ([]core.Show, error) {
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

	var shows []core.Show

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
		titleParts := parts[:len(parts)-1]
		title := strings.TrimSpace(strings.Join(titleParts, " "))

		// Parse date (format: 10/12 or 1/5)
		date, err := parseMercuryEastDate(datePart, currentYear)
		if err != nil {
			fmt.Printf("    Warning: could not parse date '%s' for %s\n", datePart, title)
			continue
		}

		shows = append(shows, core.Show{
			Title:    title,
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
