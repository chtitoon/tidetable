package scrapers

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/chtitoon/tidetable/lescraper/core"
)

// BearsvilleTheater scrapes the Bearsville Theater (Woodstock), a WordPress
// (Divi + Rockhouse/Etix "RHP Events") site. The homepage is the full
// "Upcoming Shows" list, server-rendered in one page with no pagination: the
// cards inside the page's post-content block. The slideshow and "Featured
// Shows" blocks above it repeat some of those cards and are skipped.
//
// There is no JSON source: the events post type isn't exposed in the REST
// API, the events RSS feed carries publish dates rather than show dates, and
// Etix answers plain requests with an empty bot-check response. Cards give
// "Dec 5" without a year, so the year is inferred: the list is chronological,
// so it starts at the current year (or the next one, for a date well in the
// past) and moves on whenever the month goes backwards. Ticket status isn't
// published anywhere on the site, so it is always unknown.
type BearsvilleTheater struct{}

const bearsvilleURL = "https://bearsvilletheater.com/"

func init() { core.RegisterVenue("Bearsville Theater", &BearsvilleTheater{}) }

func (s *BearsvilleTheater) GetVenues() []string { return []string{"Bearsville Theater"} }
func (s *BearsvilleTheater) GetName() string     { return "Bearsville Theater (WordPress)" }
func (s *BearsvilleTheater) GetURL() string      { return bearsvilleURL }

func (s *BearsvilleTheater) ScrapeShows() ([]core.Show, error) {
	body, err := fetchURL(bearsvilleURL)
	if err != nil {
		return nil, err
	}
	return parseBearsville(body, now())
}

var (
	// "Show: 8:00 PM" / "Doors: 7:00 PM".
	bearsvilleClockRe = regexp.MustCompile(`(?i)(\d{1,2}:\d{2}\s*[ap]m)`)
	// Description lines that name the support act: "with Tyler Ramsey", "w/ Tyler Ballgame".
	bearsvilleSupportRe = regexp.MustCompile(`(?i)^(?:with|w/)\s+(.+)$`)
	bearsvilleCancelRe  = regexp.MustCompile(`(?i)\b(?:cancell?ed|postponed)\b`)
)

func parseBearsville(body []byte, today time.Time) ([]core.Show, error) {
	doc, err := parseHTML(body)
	if err != nil {
		return nil, err
	}
	list := find(doc, byClass("et_pb_post_content"))
	if list == nil {
		return nil, fmt.Errorf("no .et_pb_post_content block; has the homepage layout changed?")
	}
	cards := findAll(list, byClass("bv-event-wrapper"))
	if len(cards) == 0 {
		return nil, fmt.Errorf("no .bv-event-wrapper cards in the upcoming list; has the homepage layout changed?")
	}

	today = today.In(newYork)
	year := today.Year()
	prevMonth := time.Month(0)
	var shows []core.Show
	for i, c := range cards {
		link := find(c, byClass("bv-event-url"))
		monthDay := textOf(c, "bv-event-monthday")
		if link == nil || monthDay == "" {
			return nil, fmt.Errorf("card %d: missing title link or date", i+1)
		}
		md, err := parseMonthDay(monthDay)
		if err != nil {
			return nil, fmt.Errorf("card %d: %w", i+1, err)
		}
		if prevMonth == 0 {
			year = nearestYear(md, today)
		} else if md.Month() < prevMonth {
			year++
		}
		prevMonth = md.Month()
		date := time.Date(year, md.Month(), md.Day(), 0, 0, 0, 0, newYork)

		title := cleanText(html.UnescapeString(text(link)))
		subtitle := textOf(c, "bv-event-subtitle")
		desc := textOf(c, "bv-event-description")
		if bearsvilleCancelRe.MatchString(title + " " + subtitle + " " + desc) {
			continue
		}

		show := core.Show{
			Title:    title,
			Venue:    "Bearsville Theater",
			Date:     date,
			Time:     bearsvilleTime(textOf(c, "bv-event-show"), textOf(c, "bv-event-doors")),
			Status:   core.StatusUnknown,
			URL:      attr(link, "href"),
			VenueURL: bearsvilleURL,
		}
		if t := find(c, byClass("bv-cta-tickets")); t != nil && attr(t, "href") != "" {
			show.URL = attr(t, "href")
		}
		if m := bearsvilleSupportRe.FindStringSubmatch(desc); m != nil {
			show.Support = []string{m[1]}
		}
		shows = append(shows, show)
	}
	return shows, nil
}

// parseMonthDay parses "Dec 5" or "December 5" (year 0).
func parseMonthDay(s string) (time.Time, error) {
	s = strings.ReplaceAll(s, "Sept ", "Sep ")
	for _, layout := range []string{"Jan 2", "January 2"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised date %q", s)
}

// nearestYear returns the year that puts month/day closest to today, so the
// first card of the list (normally within days of today, possibly a day
// stale) lands in the right year across New Year.
func nearestYear(md, today time.Time) int {
	best, bestGap := today.Year(), time.Duration(1<<63-1)
	for y := today.Year() - 1; y <= today.Year()+1; y++ {
		gap := time.Date(y, md.Month(), md.Day(), 0, 0, 0, 0, newYork).Sub(today)
		if gap < 0 {
			gap = -gap
		}
		if gap < bestGap {
			best, bestGap = y, gap
		}
	}
	return best
}

// bearsvilleTime returns the show time as "20:00", falling back to doors when
// no show time is listed, or "" when neither is.
func bearsvilleTime(show, doors string) string {
	for _, s := range []string{show, doors} {
		if m := bearsvilleClockRe.FindString(s); m != "" {
			if t, err := time.Parse("3:04PM", strings.ToUpper(strings.ReplaceAll(m, " ", ""))); err == nil {
				return t.Format("15:04")
			}
		}
	}
	return ""
}
