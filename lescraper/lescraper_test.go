package lescraper

import (
	"errors"
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
		{Title: "Before window", Date: d(11, 20)},       // dropped
		{Title: "After window", Date: d(12, 31)},        // dropped
		{Title: "DJ Dance Party", Date: d(12, 3)},       // non-concert
		{Title: "Same title, other room", Date: d(12, 1), Time: "21:00", Room: "Studio"},
	}})
	RegisterVenue("test broken", &fakeScraper{name: "test broken", err: errors.New("HTTP 503")})
	RegisterVenue("test empty", &fakeScraper{name: "test empty", shows: []core.Show{{Title: "Too late", Date: d(12, 31)}}})
	RegisterVenue("test panics", &fakeScraper{name: "test panics", panic: true})

	results, err := ScrapeVenues([]string{"test ok", "test broken", "test empty", "test panics"}, d(11, 21), d(12, 30))
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
	if _, err := ScrapeVenues([]string{"No Such Venue"}, d(11, 21), d(12, 30)); err == nil {
		t.Error("want error for an unregistered venue")
	}
}
