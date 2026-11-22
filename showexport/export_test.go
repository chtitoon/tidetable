package showexport

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chtitoon/tidetable/lescraper"
	"github.com/chtitoon/tidetable/lescraper/core"
)

var ny, _ = time.LoadLocation("America/New_York")

func date(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, ny) }

func sampleResults() []lescraper.VenueResult {
	return []lescraper.VenueResult{
		{Name: "LPR", URL: "https://lpr.com/", Shows: []core.Show{
			{Title: "Friendship & Charlotte Cornfield", Venue: "LPR", Room: "Main Space", Date: date(12, 6), Time: "18:30",
				Status: core.StatusUnknown, URL: "https://lpr.com/lpr_events/fcc26nyc/"},
			{Title: "Catie Turner", Venue: "LPR", Room: "Stone Circle Theatre", Date: date(11, 21), Time: "19:00",
				URL: "https://lpr.com/lpr_events/ct26nyc/"},
		}},
		{Name: "Mercury Lounge", URL: "https://mercuryeastpresents.com/mercurylounge/", Err: errors.New("HTTP 503")},
		{Name: "Brooklyn Steel", URL: "https://www.bowerypresents.com/brooklyn-steel/", Shows: []core.Show{
			{Title: "Whitney", Venue: "Brooklyn Steel", Date: date(12, 6), Time: "20:00", Headliner: "Whitney",
				Support: []string{"Hand Habits"}, Status: core.StatusComingSoon,
				OnSaleAt: time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC), URL: "https://www.axs.com/events/1"},
		}},
	}
}

func TestBuild(t *testing.T) {
	doc := Build(sampleResults(), time.Date(2026, 11, 21, 10, 41, 7, 0, time.UTC))

	if doc.GeneratedAt != "2026-11-21T10:41:07Z" {
		t.Errorf("generated_at = %s", doc.GeneratedAt)
	}
	// Venues keep config order; a failed venue reports its error and 0 shows.
	if len(doc.Venues) != 3 || doc.Venues[1].OK || *doc.Venues[1].Error != "HTTP 503" || doc.Venues[1].Shows != 0 {
		t.Errorf("venues: %+v", doc.Venues)
	}
	if !doc.Venues[0].OK || doc.Venues[0].Shows != 2 || doc.Venues[0].Error != nil {
		t.Errorf("LPR venue: %+v", doc.Venues[0])
	}
	if doc.AllFailed() {
		t.Error("AllFailed with two OK venues")
	}

	// Sorted by date, then venue.
	var order []string
	for _, s := range doc.Shows {
		order = append(order, s.Date+" "+s.Venue)
	}
	if strings.Join(order, ",") != "2026-11-21 LPR,2026-12-06 Brooklyn Steel,2026-12-06 LPR" {
		t.Errorf("order: %v", order)
	}

	// Title split when the source gives no lineup; explicit lineup kept.
	fr := doc.Shows[2]
	if fr.Headliner != "Friendship" || len(fr.Support) != 1 || fr.Support[0] != "Charlotte Cornfield" ||
		fr.Title != "Friendship & Charlotte Cornfield" || *fr.Room != "Main Space" || fr.Status != "unknown" {
		t.Errorf("LPR show: %+v", fr)
	}
	wh := doc.Shows[1]
	if wh.Headliner != "Whitney" || wh.Support[0] != "Hand Habits" || *wh.OnSaleAt != "2026-12-01T15:00:00Z" ||
		wh.Room != nil || wh.Status != "coming_soon" {
		t.Errorf("Brooklyn Steel show: %+v", wh)
	}
	if doc.Shows[0].Status != "unknown" { // empty status defaults
		t.Errorf("status %q", doc.Shows[0].Status)
	}
}

func TestJSONShape(t *testing.T) {
	doc := Build(sampleResults(), time.Date(2026, 11, 21, 10, 41, 7, 0, time.UTC))
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var generic struct {
		Venues []map[string]any `json:"venues"`
		Shows  []map[string]any `json:"shows"`
	}
	if err := json.Unmarshal(b, &generic); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"name", "url", "ok", "shows", "skipped", "latest_date", "error"} {
		if _, ok := generic.Venues[0][k]; !ok {
			t.Errorf("venue missing %q", k)
		}
	}
	s := generic.Shows[0]
	for _, k := range []string{"date", "time", "venue", "room", "headliner", "support", "title", "status", "on_sale_at", "url"} {
		if _, ok := s[k]; !ok {
			t.Errorf("show missing %q", k)
		}
	}
	// Unknowns are null, support is always an array.
	if s["on_sale_at"] != nil {
		t.Errorf("on_sale_at = %v, want null", s["on_sale_at"])
	}
	if _, ok := s["support"].([]any); !ok {
		t.Errorf("support = %#v, want []", s["support"])
	}
}

func TestLatestDateAndSkipped(t *testing.T) {
	doc := Build(sampleResults(), time.Now())
	if lpr := doc.Venues[0]; lpr.LatestDate == nil || *lpr.LatestDate != "2026-12-06" {
		t.Errorf("LPR latest_date = %v, want 2026-12-06", lpr.LatestDate)
	}
	if failed := doc.Venues[1]; failed.LatestDate != nil {
		t.Errorf("failed venue latest_date = %q, want null", *failed.LatestDate)
	}

	doc = Build([]lescraper.VenueResult{{Name: "Bowery Ballroom", Skipped: 2, Shows: []core.Show{
		{Title: "MGMT", Venue: "Bowery Ballroom", Date: date(12, 1)},
	}}}, time.Now())
	if v := doc.Venues[0]; v.Skipped != 2 || v.Shows != 1 {
		t.Errorf("venue %+v, want 1 show, 2 skipped", v)
	}
}

func TestFarOutShowsStay(t *testing.T) {
	// A show 10 months out is written like any other.
	tenMonths := date(11, 21).AddDate(0, 10, 0)
	doc := Build([]lescraper.VenueResult{{Name: "Bowery Presents NYC", Shows: []core.Show{
		{Title: "Big Thief", Venue: "Bowery Presents NYC", Room: "Forest Hills Stadium", Date: tenMonths},
		{Title: "Soon", Venue: "Bowery Presents NYC", Date: date(11, 22)},
	}}}, time.Now())
	if len(doc.Shows) != 2 || doc.Shows[1].Date != "2027-09-21" || *doc.Venues[0].LatestDate != "2027-09-21" {
		t.Errorf("shows %+v, latest %v", doc.Shows, *doc.Venues[0].LatestDate)
	}
}

func TestIncompleteVenueKeepsShows(t *testing.T) {
	// A venue that stopped at its page cap: shows written, venue flagged.
	capErr := fmt.Errorf("%w: still finding events after 30 pages", core.ErrIncomplete)
	doc := Build([]lescraper.VenueResult{{Name: "Mercury Lounge", Err: capErr, Shows: []core.Show{
		{Title: "aeter", Venue: "Mercury Lounge", Date: date(11, 21)},
	}}}, time.Now())
	v := doc.Venues[0]
	if v.OK || v.Shows != 1 || v.Error == nil || !strings.Contains(*v.Error, "30 pages") || len(doc.Shows) != 1 {
		t.Errorf("venue %+v, %d shows", v, len(doc.Shows))
	}
	if doc.AllFailed() {
		t.Error("AllFailed with shows to write; the file would be kept stale")
	}
}

func TestAllFailed(t *testing.T) {
	doc := Build([]lescraper.VenueResult{{Name: "A", Err: errors.New("x")}, {Name: "B", Err: lescraper.ErrNoShows}}, time.Now())
	if !doc.AllFailed() {
		t.Error("want AllFailed")
	}
	if *doc.Venues[1].Error != "0 shows" {
		t.Errorf("error = %q", *doc.Venues[1].Error)
	}
}

func TestWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "shows.json")
	doc := Build(sampleResults(), time.Date(2026, 11, 21, 10, 41, 7, 0, time.UTC))
	if err := Write(path, doc); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var back Doc
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Shows) != 3 || !strings.HasSuffix(string(b), "}\n") {
		t.Errorf("round trip: %d shows", len(back.Shows))
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
}
