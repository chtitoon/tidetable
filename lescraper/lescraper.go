// Package lescraper provides a unified interface for scraping concert venues
// Users specify venue names, and lescraper handles the rest
package lescraper

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/chtitoon/tidetable/lescraper/core"
	// Import scrapers to trigger their init() functions which register venues
	_ "github.com/chtitoon/tidetable/lescraper/scrapers"
)

// Show is an alias for core.Show for convenience
type Show = core.Show

// NormalizedShow is an alias for core.NormalizedShow for convenience
type NormalizedShow = core.NormalizedShow

// VenueResult is the outcome of scraping one configured venue.
type VenueResult struct {
	Name    string
	URL     string
	Shows   []Show // dated today or later, sorted by date and time
	Dropped int    // listings dropped as non-concerts
	Skipped int    // listings skipped for a placeholder headliner ("@", "TBA")
	// Err is non-nil if the venue failed or listed 0 shows. If it wraps
	// core.ErrIncomplete, Shows still holds what was scraped before stopping.
	Err error
}

// ErrNoShows marks a venue that scraped fine but listed no upcoming shows. Real venues always list something, so it's treated as a failure.
var ErrNoShows = errors.New("0 shows")

// ScrapeVenues scrapes each venue (concurrently) and keeps every show dated
// today or later (calendar dates), however far out. One venue failing
// doesn't affect the others; it gets a result with Err set. The only error
// returned is for venue names with no registered scraper.
func ScrapeVenues(venueNames []string, today time.Time) ([]VenueResult, error) {
	if len(venueNames) == 0 {
		return nil, fmt.Errorf("no venues specified")
	}
	scrapers := make([]core.Scraper, len(venueNames))
	var missing []string
	for i, name := range venueNames {
		s, err := core.GetScraperForVenue(name)
		if err != nil {
			missing = append(missing, name)
			continue
		}
		scrapers[i] = s
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("unsupported venues: %v (supported: %v)", missing, core.GetSupportedVenues())
	}

	results := make([]VenueResult, len(venueNames))
	var wg sync.WaitGroup
	for i, name := range venueNames {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = scrapeOne(name, scrapers[i], today)
		}()
	}
	wg.Wait()
	return results, nil
}

func scrapeOne(name string, s core.Scraper, today time.Time) (res VenueResult) {
	res = VenueResult{Name: name, URL: s.GetURL()}
	defer func() {
		if r := recover(); r != nil { // a scraper bug mustn't take down the run
			res.Shows, res.Err = nil, fmt.Errorf("scraper panicked: %v", r)
		}
	}()

	shows, err := s.ScrapeShows()
	if err != nil && !errors.Is(err, core.ErrIncomplete) {
		res.Err = err
		return res
	}
	res.Shows, res.Dropped, res.Skipped = filterShows(name, shows, today)
	switch {
	case err != nil: // incomplete: keep what was scraped, report the venue failed
		res.Err = err
	case len(res.Shows) == 0:
		res.Err = ErrNoShows
	}
	return res
}

// filterShows keeps shows dated today or later, drops obvious non-concerts,
// shows with a placeholder headliner and duplicates, and sorts by date and
// time. It returns the counts of non-concerts dropped and placeholders skipped.
func filterShows(venue string, shows []Show, today time.Time) (out []Show, dropped, skipped int) {
	start := civil(today)
	seen := map[string]bool{}
	for _, show := range shows {
		show.Venue = venue
		d := civil(show.Date)
		if d < start {
			continue
		}
		if core.IsNonConcert(show.Title) {
			dropped++
			continue
		}
		if headliner, _ := core.Lineup(show); core.IsPlaceholderName(headliner) {
			skipped++
			continue
		}
		key := d + "|" + show.Time + "|" + show.Room + "|" + show.Title + "|" + show.URL
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, show)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if di, dj := civil(out[i].Date), civil(out[j].Date); di != dj {
			return di < dj
		}
		return out[i].Time < out[j].Time
	})
	return out, dropped, skipped
}

// civil formats t's own calendar date as YYYY-MM-DD, for comparisons that
// don't depend on time zones.
func civil(t time.Time) string { return t.Format("2006-01-02") }

// NormalizeShows processes raw shows and extracts individual artists
func NormalizeShows(shows []Show) []NormalizedShow {
	return core.NormalizeShows(shows)
}

// GetSupportedVenues returns a sorted list of all registered venue names
func GetSupportedVenues() []string {
	return core.GetSupportedVenues()
}

// RegisterVenue allows users to register custom venues with their own scrapers
// This enables extending lescraper with new venue support
func RegisterVenue(venueName string, scraper core.Scraper) {
	core.RegisterVenue(venueName, scraper)
}
