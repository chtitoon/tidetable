package lescraper

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chtitoon/tidetable/lescraper/core"
)

type fakeScraper struct {
	name  string
	shows []core.Show
	err   error
	panic bool
}

func (f *fakeScraper) GetVenues() []string { return []string{f.name} }
func (f *fakeScraper) GetName() string     { return f.name }
func (f *fakeScraper) GetURL() string      { return "https://example.com/" + f.name }
func (f *fakeScraper) ScrapeShows() ([]core.Show, error) {
	if f.panic {
		panic("boom")
	}
	return f.shows, f.err
}

var ny, _ = time.LoadLocation("America/New_York")

func d(m time.Month, day int) time.Time { return time.Date(2026, m, day, 0, 0, 0, 0, ny) }

func TestScrapeVenuesIsolatesFailures(t *testing.T) {
	RegisterVenue("test ok", &fakeScraper{name: "test ok", shows: []core.Show{
		{Title: "Later", Date: d(12, 2), Time: "20:00"},
		{Title: "Early", Date: d(12, 1), Time: "21:00"},
		{Title: "Early", Date: d(12, 1), Time: "21:00"}, // duplicate
		{Title: "Before today", Date: d(11, 20)},        // dropped
		{Title: "DJ Dance Party", Date: d(12, 3)},       // non-concert
		{Title: "Same title, other room", Date: d(12, 1), Time: "21:00", Room: "Studio"},
	}})
	RegisterVenue("test broken", &fakeScraper{name: "test broken", err: errors.New("HTTP 503")})
	RegisterVenue("test empty", &fakeScraper{name: "test empty", shows: []core.Show{{Title: "Already played", Date: d(11, 20)}}})
	RegisterVenue("test panics", &fakeScraper{name: "test panics", panic: true})

	results, err := ScrapeVenues([]string{"test ok", "test broken", "test empty", "test panics"}, d(11, 21))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 4 {
		t.Fatalf("got %d results", len(results))
	}

	ok := results[0]
	if ok.Err != nil || ok.Name != "test ok" || ok.URL != "https://example.com/test ok" {
		t.Fatalf("ok venue: %+v", ok)
	}
	var titles []string
	for _, s := range ok.Shows {
		titles = append(titles, s.Title+"@"+s.Room)
		if s.Venue != "test ok" {
			t.Errorf("venue not set: %+v", s)
		}
	}
	want := []string{"Early@", "Same title, other room@Studio", "Later@"}
	if len(titles) != len(want) {
		t.Fatalf("shows %v, want %v", titles, want)
	}
	for i := range want {
		if titles[i] != want[i] {
			t.Errorf("shows %v, want %v", titles, want)
			break
		}
	}
	if ok.Dropped != 1 {
		t.Errorf("dropped %d, want 1", ok.Dropped)
	}

	if results[1].Err == nil || results[1].Err.Error() != "HTTP 503" {
		t.Errorf("broken venue err = %v", results[1].Err)
	}
	if !errors.Is(results[2].Err, ErrNoShows) || results[2].Err.Error() != "0 shows" {
		t.Errorf("empty venue err = %v, want \"0 shows\"", results[2].Err)
	}
	if results[3].Err == nil {
		t.Errorf("panicking venue should fail, not crash")
	}
}

func TestScrapeVenuesUnknownName(t *testing.T) {
	if _, err := ScrapeVenues([]string{"No Such Venue"}, d(11, 21)); err == nil {
		t.Error("want error for an unregistered venue")
	}
}

func TestNoDateCutoff(t *testing.T) {
	// Shows far out (10 months, 2 years) stay; only past dates go.
	tenMonths := d(11, 21).AddDate(0, 10, 0)
	RegisterVenue("test far", &fakeScraper{name: "test far", shows: []core.Show{
		{Title: "Big Thief", Date: tenMonths, Time: "18:30"},
		{Title: "Way Out", Date: d(11, 21).AddDate(2, 0, 0)},
		{Title: "Tonight", Date: d(11, 21)},
		{Title: "Yesterday", Date: d(11, 20)},
	}})
	results, err := ScrapeVenues([]string{"test far"}, d(11, 21))
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, s := range results[0].Shows {
		titles = append(titles, s.Title)
	}
	if got := strings.Join(titles, ","); got != "Tonight,Big Thief,Way Out" {
		t.Errorf("shows %s, want Tonight,Big Thief,Way Out", got)
	}
}

func TestPlaceholderHeadlinersSkipped(t *testing.T) {
	RegisterVenue("test placeholders", &fakeScraper{name: "test placeholders", shows: []core.Show{
		{Title: "@", Date: d(12, 1)},
		{Title: "@", Date: d(12, 2), Support: []string{"June McDoom"}}, // headliner still "@"
		{Title: "TBA", Date: d(12, 3)},
		{Title: "Special Guests", Date: d(12, 4)},
		{Title: "Whatever the listing says", Headliner: "To Be Announced", Date: d(12, 5)},
		{Title: "MGMT", Date: d(12, 6)},
		{Title: "IDLES", Date: d(12, 7), Support: []string{"TBA", "@", "Special Guest(s)", "Squid"}},
	}})
	results, err := ScrapeVenues([]string{"test placeholders"}, d(11, 21))
	if err != nil {
		t.Fatal(err)
	}
	r := results[0]
	if r.Err != nil || r.Skipped != 5 || len(r.Shows) != 2 {
		t.Fatalf("got %d shows, %d skipped, err %v; want 2 shows, 5 skipped", len(r.Shows), r.Skipped, r.Err)
	}
	if r.Shows[0].Title != "MGMT" || r.Shows[1].Title != "IDLES" {
		t.Errorf("kept %q, %q", r.Shows[0].Title, r.Shows[1].Title)
	}
	if _, support := core.Lineup(r.Shows[1]); len(support) != 1 || support[0] != "Squid" {
		t.Errorf("IDLES support %q, want [Squid]", support)
	}
}

func TestIncompleteListingKeepsShows(t *testing.T) {
	// A scraper that hit its page cap returns what it got plus ErrIncomplete:
	// the shows are kept and the venue is still flagged.
	capErr := fmt.Errorf("%w: still finding events after 30 pages", core.ErrIncomplete)
	RegisterVenue("test capped", &fakeScraper{name: "test capped", err: capErr, shows: []core.Show{
		{Title: "Found before the cap", Date: d(12, 1)},
	}})
	results, err := ScrapeVenues([]string{"test capped"}, d(11, 21))
	if err != nil {
		t.Fatal(err)
	}
	r := results[0]
	if !errors.Is(r.Err, core.ErrIncomplete) || len(r.Shows) != 1 {
		t.Errorf("got %d shows, err %v; want 1 show and ErrIncomplete", len(r.Shows), r.Err)
	}
}
