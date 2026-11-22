package scrapers

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/chtitoon/tidetable/lescraper/core"
)

// BoweryPresents scrapes venues on The Bowery Presents / AEG sites
// (bowerypresents.com, websterhall.com). Those pages render their lists
// client-side from a static JSON feed on AEG's blob storage. "Load More" only
// reveals the next 100 events of the already-downloaded feed (list_view mode
// in the site's app.js), so one fetch has everything; a feed holding fewer
// events than its meta.total is reported as incomplete. The feed URL is the
// data-file attribute of the page's "All Shows" list.
type BoweryPresents struct {
	Name string
	URL  string // listings page the feed backs
	Feed string // JSON feed URL
	// Rooms maps the feed's venue IDs to keep to a room label. Leave the label
	// empty for single-room venues.
	Rooms map[string]string
}

// The full Bowery Presents feed (all regions). The New York Metro calendar
// and the venue pages all read it and filter client-side by venue ID.
const boweryPresentsFeed = "https://aegwebprod.blob.core.windows.net/json/events/59/events.json"

var boweryPresentsVenues = []*BoweryPresents{
	{
		Name:  "Music Hall of Williamsburg",
		URL:   "https://www.bowerypresents.com/music-hall-of-williamsburg/",
		Feed:  boweryPresentsFeed,
		Rooms: map[string]string{"125933": ""},
	},
	{
		Name:  "Brooklyn Steel",
		URL:   "https://www.bowerypresents.com/brooklyn-steel/",
		Feed:  boweryPresentsFeed,
		Rooms: map[string]string{"126144": ""},
	},
	{
		Name:  "Webster Hall",
		URL:   "https://www.websterhall.com/shows/",
		Feed:  "https://aegwebprod.blob.core.windows.net/json/events/85/events.json",
		Rooms: map[string]string{"101385": ""},
	},
	{
		// The calendar mixes regions; keep only these NYC rooms.
		Name: "Bowery Presents NYC",
		URL:  "https://www.bowerypresents.com/new-york-metro/calendar/",
		Feed: boweryPresentsFeed,
		Rooms: map[string]string{
			"100784": "Beacon Theatre",
			"125269": "Kings Theatre",
			"125705": "Terminal 5",
			"124944": "Forest Hills Stadium",
			"101171": "Radio City Music Hall",
			"128735": "Racket",
			"126624": "Knockdown Center",
			"125765": "Prospect Park Bandshell", // "BRIC Celebrate Brooklyn! Festival at Prospect Park Bandshell"
			"101055": "Madison Square Garden",
			"101308": "Infosys Theater at Madison Square Garden",
		},
	},
}

func init() {
	for _, v := range boweryPresentsVenues {
		core.RegisterVenue(v.Name, v)
	}
}

func (s *BoweryPresents) GetVenues() []string { return []string{s.Name} }
func (s *BoweryPresents) GetName() string     { return "Bowery Presents feed: " + s.Name }
func (s *BoweryPresents) GetURL() string      { return s.URL }

func (s *BoweryPresents) ScrapeShows() ([]core.Show, error) {
	body, err := fetchShared(s.Feed)
	if err != nil {
		return nil, err
	}
	return parseBoweryPresentsFeed(body, s)
}

type aegFeed struct {
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
	Events []aegEvent `json:"events"`
}

type aegEvent struct {
	EventID string `json:"eventId"`
	Active  bool   `json:"active"`
	Private bool   `json:"private"`
	Title   struct {
		Headliners     string `json:"headliners"` // HTML, artists linked
		Supporting     string `json:"supportingText"`
		EventTitleText string `json:"eventTitleText"`
	} `json:"title"`
	EventDateTime     string `json:"eventDateTime"` // local, "2026-11-22T20:00:00" or "TBD"
	OnsaleDateTimeUTC string `json:"onsaleDateTimeUTC"`
	Ticketing         struct {
		StatusID int    `json:"statusId"`
		Status   string `json:"status"`
		URL      string `json:"url"`
		EventURL string `json:"eventUrl"`
	} `json:"ticketing"`
	Venue struct {
		VenueID string `json:"venueId"`
		Title   string `json:"title"`
	} `json:"venue"`
}

// AEG ticketing.statusId values seen in the feed.
const (
	aegStatusCancelled = 2
	aegStatusPostponed = 5
	aegStatusSoldOut   = 7
)

var anchorTextRe = regexp.MustCompile(`(?s)<a\b[^>]*>(.*?)</a>`)

func parseBoweryPresentsFeed(body []byte, s *BoweryPresents) ([]core.Show, error) {
	var feed aegFeed
	if err := json.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("decoding feed: %w", err)
	}
	if len(feed.Events) == 0 {
		return nil, fmt.Errorf("feed has no events (meta.total=%d)", feed.Meta.Total)
	}

	var shows []core.Show
	for _, e := range feed.Events {
		room, ok := s.Rooms[e.Venue.VenueID]
		if !ok || !e.Active || e.Private {
			continue
		}
		status := strings.ToLower(e.Ticketing.Status)
		if e.Ticketing.StatusID == aegStatusCancelled || e.Ticketing.StatusID == aegStatusPostponed ||
			strings.Contains(status, "cancel") || strings.Contains(status, "postpone") {
			continue
		}
		start, err := time.ParseInLocation("2006-01-02T15:04:05", e.EventDateTime, newYork)
		if err != nil {
			continue // "TBD": no date yet
		}

		show := core.Show{
			Title:    cleanText(html.UnescapeString(e.Title.EventTitleText)),
			Venue:    s.Name,
			Room:     room,
			Date:     dateOnly(start),
			Status:   core.StatusOnSale,
			URL:      firstNonEmpty(e.Ticketing.EventURL, e.Ticketing.URL),
			VenueURL: s.URL,
		}
		if start.Hour() != 0 || start.Minute() != 0 { // midnight = time not announced
			show.Time = clock(start)
		}
		if onsale, err := time.Parse("2006-01-02T15:04:05", e.OnsaleDateTimeUTC); err == nil {
			show.OnSaleAt = onsale
		}
		switch {
		case e.Ticketing.StatusID == aegStatusSoldOut || strings.Contains(status, "sold out"):
			show.Status = core.StatusSoldOut
		case strings.Contains(status, "coming soon") || (!show.OnSaleAt.IsZero() && show.OnSaleAt.After(now())):
			show.Status = core.StatusComingSoon
		}

		// Linked names in the headliners HTML are the artists; the rest is
		// decoration ("<a>Jamila Woods</a>: 10 Years of HEAVN").
		var linked []string
		for _, m := range anchorTextRe.FindAllStringSubmatch(e.Title.Headliners, -1) {
			if name := cleanText(html.UnescapeString(stripTags(m[1]))); name != "" {
				linked = append(linked, name)
			}
		}
		if len(linked) > 0 {
			show.Headliner = linked[0]
			show.Support = linked[1:]
		}
		show.Support = append(show.Support, splitCommaList(html.UnescapeString(e.Title.Supporting))...)

		shows = append(shows, show)
	}
	if len(feed.Events) < feed.Meta.Total {
		return shows, fmt.Errorf("%w: feed lists %d of %d events", core.ErrIncomplete, len(feed.Events), feed.Meta.Total)
	}
	return shows, nil
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

func stripTags(s string) string { return tagRe.ReplaceAllString(s, " ") }

// splitCommaList splits "A, B, and C" into ["A", "B", "C"].
func splitCommaList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = cleanText(p)
		p = strings.TrimSpace(strings.TrimPrefix(p, "and "))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
