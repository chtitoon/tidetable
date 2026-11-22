package scrapers

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/chtitoon/tidetable/lescraper/core"
	"golang.org/x/net/html"
)

// MercuryEast scrapes a Mercury East Presents venue (Mercury Lounge, Bowery
// Ballroom). Their list views are server-rendered WordPress pages, 20 events
// per page at <url>/page/N/; a page past the end renders with no events.
type MercuryEast struct {
	Name string
	URL  string
}

var mercuryEastVenues = []*MercuryEast{
	{Name: "Mercury Lounge", URL: "https://mercuryeastpresents.com/mercurylounge/"},
	{Name: "Bowery Ballroom", URL: "https://mercuryeastpresents.com/boweryballroom/"},
}

// mercuryEastMaxPages bounds pagination in case the end-of-list check breaks.
const mercuryEastMaxPages = 20

func init() {
	for _, v := range mercuryEastVenues {
		core.RegisterVenue(v.Name, v)
	}
}

func (s *MercuryEast) GetVenues() []string { return []string{s.Name} }
func (s *MercuryEast) GetName() string     { return "Mercury East Presents: " + s.Name }
func (s *MercuryEast) GetURL() string      { return s.URL }

// ScrapeShows walks the list pages until one has no events.
func (s *MercuryEast) ScrapeShows() ([]core.Show, error) {
	var all []core.Show
	for page := 1; page <= mercuryEastMaxPages; page++ {
		url := s.URL
		if page > 1 {
			url = fmt.Sprintf("%spage/%d/", s.URL, page)
		}
		body, err := fetchURL(url)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		shows, err := parseMercuryEastPage(body, s.Name, s.URL)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		if len(shows) == 0 {
			return all, nil
		}
		all = append(all, shows...)
	}
	return nil, fmt.Errorf("still finding events after %d pages; pagination check may be broken", mercuryEastMaxPages)
}

var (
	// "*SOLD OUT* cortex", "*POSTPONED* Artist"
	mercuryMarkerRe = regexp.MustCompile(`^\s*\*([^*]+)\*\s*`)
	// "Doors: 6:00 pm"
	clockRe = regexp.MustCompile(`(?i)(\d{1,2}):(\d{2})\s*([ap])\.?m\.?`)
)

// parseMercuryEastPage extracts the events on one list page. Sidebar
// widgets reuse .tw-section without a date, so sections need .tw-event-date.
func parseMercuryEastPage(body []byte, venue, venueURL string) ([]core.Show, error) {
	doc, err := parseHTML(body)
	if err != nil {
		return nil, err
	}
	var shows []core.Show
	for _, sec := range findAll(doc, byClass("tw-section")) {
		dateText := textOf(sec, "tw-event-date")
		if dateText == "" {
			continue
		}
		date, err := time.ParseInLocation("Mon Jan 2, 2006", dateText, newYork)
		if err != nil {
			return nil, fmt.Errorf("parsing date %q: %w", dateText, err)
		}

		nameNode := find(sec, byClass("tw-name"))
		title := text(nameNode)
		status := core.StatusUnknown
		for {
			m := mercuryMarkerRe.FindStringSubmatch(title)
			if m == nil {
				break
			}
			switch marker := strings.ToUpper(strings.TrimSpace(m[1])); {
			case strings.Contains(marker, "SOLD OUT"):
				status = core.StatusSoldOut
			case strings.Contains(marker, "POSTPONED"), strings.Contains(marker, "CANCEL"):
				status = "" // drop
			}
			title = title[len(m[0]):]
		}
		if status == "" || title == "" {
			continue
		}
		if status == core.StatusUnknown {
			status = mercuryEastButtonStatus(sec)
		}

		url := ""
		if a := find(nameNode, byTag("a")); a != nil {
			url = attr(a, "href")
		}
		if url == "" {
			if btn := find(sec, byClass("tw-buy-tix-btn")); btn != nil {
				url = attr(btn, "href")
			}
		}

		shows = append(shows, core.Show{
			Title:    title,
			Venue:    venue,
			Date:     date,
			Time:     parseClock(textOf(sec, "tw-event-time")),
			Support:  mercuryEastSupport(find(sec, byClass("tw-attractions"))),
			Status:   status,
			URL:      url,
			VenueURL: venueURL,
		})
	}
	return shows, nil
}

// mercuryEastButtonStatus maps the ticket button's class. The button markup
// also appears in an HTML comment ("Onsale Soon"); the parser skips comments.
func mercuryEastButtonStatus(sec *html.Node) core.Status {
	btn := find(sec, byClass("tw-buy-tix-btn"))
	switch {
	case btn == nil:
		return core.StatusUnknown
	case hasClass(btn, "tw_soldout"):
		return core.StatusSoldOut
	case hasClass(btn, "tw_onsale"):
		return core.StatusOnSale
	case hasClass(btn, "tw_onsalesoon"), hasClass(btn, "tw_signup"):
		return core.StatusComingSoon
	default: // tw_offsale ("Sales Ended") and anything new
		return core.StatusUnknown
	}
}

// mercuryEastSupport reads the "with X, Y" line. Each act is its own <span>.
func mercuryEastSupport(n *html.Node) []string {
	if n == nil {
		return nil
	}
	var out []string
	for _, sp := range findAll(n, byTag("span")) {
		if t := text(sp); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 { // plain text fallback: "with X, Y"
		t := strings.TrimSpace(strings.TrimPrefix(text(n), "with "))
		for _, p := range strings.Split(t, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// parseClock turns "Doors: 6:00 pm" or "7:00PM" into "18:00"; "" if none.
func parseClock(s string) string {
	m := clockRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	h := 0
	fmt.Sscanf(m[1], "%d", &h)
	if h < 1 || h > 12 {
		return ""
	}
	if strings.EqualFold(m[3], "p") && h != 12 {
		h += 12
	}
	if strings.EqualFold(m[3], "a") && h == 12 {
		h = 0
	}
	return fmt.Sprintf("%02d:%s", h, m[2])
}
