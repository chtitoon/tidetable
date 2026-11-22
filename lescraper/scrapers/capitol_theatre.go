package scrapers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/chtitoon/tidetable/lescraper/core"
)

// CapitolTheatre scrapes The Capitol Theatre and Garcia's (Port Chester), a
// Live Nation venue site (Next.js). The old /events/venue/... URLs redirect
// to /shows. The visible grid renders client-side, but each page's HTML
// carries the full event objects in its React Server Components payload
// (self.__next_f.push chunks) as SWR fallback data, with status, lineup and
// segment. /shows is capped at 36 events, so each room's page is read
// instead (/shows/rooms/<slug>), which holds that room's whole list.
type CapitolTheatre struct {
	Rooms []capitolRoom
}

type capitolRoom struct {
	Name string
	URL  string
}

const capitolURL = "https://www.thecapitoltheatre.com/shows"

func init() {
	core.RegisterVenue("The Capitol Theatre", &CapitolTheatre{Rooms: []capitolRoom{
		{Name: "The Capitol Theatre", URL: "https://www.thecapitoltheatre.com/shows/rooms/the-capitol-theatre"},
		{Name: "Garcia's", URL: "https://www.thecapitoltheatre.com/shows/rooms/garcia-s"},
	}})
}

func (s *CapitolTheatre) GetVenues() []string { return []string{"The Capitol Theatre"} }
func (s *CapitolTheatre) GetName() string     { return "The Capitol Theatre (Live Nation)" }
func (s *CapitolTheatre) GetURL() string      { return capitolURL }

func (s *CapitolTheatre) ScrapeShows() ([]core.Show, error) {
	var all []core.Show
	seen := map[string]bool{}
	for _, room := range s.Rooms {
		body, err := fetchURL(room.URL)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", room.Name, err)
		}
		events, limit, err := parseLiveNationEvents(body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", room.Name, err)
		}
		if limit > 0 && len(events) >= limit {
			fmt.Printf("    Warning: %s page returned %d events, its page size; later shows may be missing\n", room.Name, len(events))
		}
		for _, sh := range liveNationShows(events, "The Capitol Theatre", room) {
			key := sh.URL + sh.Date.String() + sh.Time + sh.Title
			if !seen[key] {
				seen[key] = true
				all = append(all, sh)
			}
		}
	}
	return all, nil
}

type lnEvent struct {
	ID             any    `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	StartDateLocal string `json:"start_date_local"`
	StartTimeLocal string `json:"start_time_local"`
	StatusCode     string `json:"status_code"`
	Segment        string `json:"segment"`
	SalesStartUTC  string `json:"sales_start_datetime_utc"`
	Artists        []struct {
		Name string `json:"name"`
	} `json:"artists"`
	VenueRoom struct {
		Name string `json:"name"`
	} `json:"venue_room"`
}

var rscChunkRe = regexp.MustCompile(`(?s)self\.__next_f\.push\((\[.*?\])\)</script>`)
var rscLineRe = regexp.MustCompile(`^[0-9a-f]+:(.*)$`)

// parseLiveNationEvents decodes the RSC payload and returns the first event
// list it finds, plus the page size from its filter ([room, offset, limit, ...]).
func parseLiveNationEvents(body []byte) ([]lnEvent, int, error) {
	var payload strings.Builder
	for _, m := range rscChunkRe.FindAllSubmatch(body, -1) {
		var chunk []any
		if err := json.Unmarshal(m[1], &chunk); err != nil {
			continue
		}
		if len(chunk) > 1 {
			if s, ok := chunk[1].(string); ok {
				payload.WriteString(s)
			}
		}
	}
	if payload.Len() == 0 {
		return nil, 0, fmt.Errorf("no Next.js RSC payload in page")
	}
	for _, line := range strings.Split(payload.String(), "\n") {
		m := rscLineRe.FindStringSubmatch(line)
		if m == nil || (!strings.HasPrefix(m[1], "[") && !strings.HasPrefix(m[1], "{")) {
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(m[1]), &v); err != nil {
			continue
		}
		if holder := findEventList(v); holder != nil {
			raw, _ := json.Marshal(holder["data"])
			var events []lnEvent
			if err := json.Unmarshal(raw, &events); err != nil {
				return nil, 0, fmt.Errorf("decoding events: %w", err)
			}
			limit := 0
			if f, ok := holder["filter"].([]any); ok && len(f) > 2 {
				if n, ok := f[2].(float64); ok {
					limit = int(n)
				}
			}
			return events, limit, nil
		}
	}
	return nil, 0, fmt.Errorf("no event list in RSC payload")
}

// findEventList finds {"data": [ {start_date_local: ...}, ... ], ...}.
func findEventList(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		if data, ok := t["data"].([]any); ok {
			if len(data) > 0 {
				if first, ok := data[0].(map[string]any); ok {
					if _, ok := first["start_date_local"]; ok {
						return t
					}
				}
			} else if _, ok := t["filter"]; ok {
				return t // an empty room still has a (empty) list
			}
		}
		for _, c := range t {
			if r := findEventList(c); r != nil {
				return r
			}
		}
	case []any:
		for _, c := range t {
			if r := findEventList(c); r != nil {
				return r
			}
		}
	}
	return nil
}

func liveNationShows(events []lnEvent, venue string, room capitolRoom) []core.Show {
	var shows []core.Show
	for _, e := range events {
		// Ticketmaster segments: "Music", "Arts & Theatre" (dance, comedy)...
		if e.Segment != "" && e.Segment != "Music" {
			continue
		}
		code := strings.ToLower(e.StatusCode)
		if strings.Contains(code, "cancel") || strings.Contains(code, "postpone") {
			continue
		}
		date, err := time.ParseInLocation("2006-01-02", e.StartDateLocal, newYork)
		if err != nil {
			continue
		}
		show := core.Show{
			Title:    cleanText(e.Name),
			Venue:    venue,
			Room:     firstNonEmpty(e.VenueRoom.Name, room.Name),
			Date:     date,
			Status:   core.StatusUnknown,
			URL:      firstNonEmpty(e.URL, room.URL),
			VenueURL: room.URL,
		}
		// 00:00:01 marks multi-day passes; no real start time.
		if t, err := time.Parse("15:04:05", e.StartTimeLocal); err == nil && (t.Hour() != 0 || t.Minute() != 0) {
			show.Time = t.Format("15:04")
		}
		if t, err := time.Parse(time.RFC3339, e.SalesStartUTC); err == nil {
			show.OnSaleAt = t.UTC()
		}
		switch {
		case strings.Contains(code, "sold"):
			show.Status = core.StatusSoldOut
		case !show.OnSaleAt.IsZero() && show.OnSaleAt.After(now()):
			show.Status = core.StatusComingSoon
		case code == "onsale":
			show.Status = core.StatusOnSale
		}
		for i, a := range e.Artists {
			if name := cleanText(a.Name); name != "" {
				if i == 0 {
					show.Headliner = name
				} else {
					show.Support = append(show.Support, name)
				}
			}
		}
		shows = append(shows, show)
	}
	return shows
}
