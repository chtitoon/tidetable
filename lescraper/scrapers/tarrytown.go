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

// Tarrytown scrapes Tarrytown Music Hall's ShoWare (accesso) box office. The
// page's "Upcoming" list is filled client-side from performancelist.asp,
// which returns JSON. Category 39 is "Music"; asking for it server-side
// leaves out comedy, film, talks, dance and the parking/vendor listings.
type Tarrytown struct{}

const (
	tarrytownURL  = "https://tickets.tarrytownmusichall.org/"
	tarrytownBase = "https://tickets.tarrytownmusichall.org"
	tarrytownFeed = tarrytownBase + "/include/widgets/events/performancelist.asp?action=perf&listPageSize=100&listMaxSize=0&category=39&page=%d"
)

func init() {
	core.RegisterVenue("Tarrytown Music Hall", &Tarrytown{})
}

func (s *Tarrytown) GetVenues() []string { return []string{"Tarrytown Music Hall"} }
func (s *Tarrytown) GetName() string     { return "Tarrytown Music Hall (ShoWare)" }
func (s *Tarrytown) GetURL() string      { return tarrytownURL }

func (s *Tarrytown) ScrapeShows() ([]core.Show, error) {
	var all []core.Show
	for page := 1; page <= maxPages; page++ {
		body, err := fetchURL(fmt.Sprintf(tarrytownFeed, page))
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		shows, end, err := parseTarrytown(body)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		all = append(all, shows...)
		if end {
			return all, nil
		}
	}
	return all, errPageCap()
}

type showarePerformance struct {
	EventID              int    `json:"EventID"`
	PerformanceID        int    `json:"PerformanceID"`
	Event                string `json:"Event"`
	PerformanceName      string `json:"PerformanceName"`
	PerformanceDateTime  string `json:"PerformanceDateTime"` // "Thursday, December 3, 2026 8:00:00 PM", venue time
	SalesStart           string `json:"SalesStart"`
	DisplayIcon          string `json:"DisplayIcon"`
	InternalStatus       int    `json:"InternalStatus"`
	IsSalesPeriodStarted int    `json:"IsSalesPeriodStarted"`
	IsSalesPeriodEnded   int    `json:"IsSalesPeriodEnded"`
}

const showareTime = "Monday, January 2, 2006 3:04:05 PM"

var (
	// Trailing "(12/3/26)" / "(12/20 & 12/21)" date notes in event names.
	showareDateSuffixRe = regexp.MustCompile(`\s*\([^)]*\d{1,2}/\d{1,2}[^)]*\)\s*$`)
	// "MH Studio: Bill Kelly" is in the Music Hall's studio space.
	showareStudioRe = regexp.MustCompile(`^MH Studio:\s*`)
)

func parseTarrytown(body []byte) (shows []core.Show, endOfList bool, err error) {
	var page struct {
		Performance []showarePerformance `json:"performance"`
		Settings    struct {
			EndOfList bool `json:"endOfList"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, false, fmt.Errorf("decoding performance list: %w", err)
	}
	for _, p := range page.Performance {
		start, err := time.ParseInLocation(showareTime, p.PerformanceDateTime, newYork)
		if err != nil {
			return nil, false, fmt.Errorf("parsing %q: %w", p.PerformanceDateTime, err)
		}
		title := cleanText(html.UnescapeString(p.Event))
		title = showareDateSuffixRe.ReplaceAllString(title, "")
		room := ""
		if showareStudioRe.MatchString(title) {
			title = showareStudioRe.ReplaceAllString(title, "")
			room = "MH Studio"
		}
		// The performance name says when a performance is a school show,
		// a parking pass etc.; let the shared non-concert filter see it.
		if perf := cleanText(p.PerformanceName); core.IsNonConcert(perf) {
			continue
		}

		show := core.Show{
			Title:    title,
			Venue:    "Tarrytown Music Hall",
			Room:     room,
			Date:     dateOnly(start),
			Time:     clock(start),
			URL:      fmt.Sprintf("%s/eventperformances.asp?evt=%d", tarrytownBase, p.EventID),
			VenueURL: tarrytownURL,
		}
		if t, err := time.ParseInLocation(showareTime, p.SalesStart, newYork); err == nil {
			show.OnSaleAt = t.UTC()
		}
		icon := strings.ToLower(p.DisplayIcon)
		switch {
		case strings.Contains(icon, "sold_out") || p.InternalStatus == 4:
			show.Status = core.StatusSoldOut
		case p.IsSalesPeriodStarted == 0:
			show.Status = core.StatusComingSoon
		case icon == "buy_tickets" && p.IsSalesPeriodEnded == 0:
			show.Status = core.StatusOnSale
		default: // phone-only sales, sales ended
			show.Status = core.StatusUnknown
		}
		shows = append(shows, show)
	}
	return shows, page.Settings.EndOfList || len(page.Performance) == 0, nil
}
