package scrapers

// Tests run each scraper end to end against pages saved from the venues on
// 2026-11-21 (testdata/*.gz); fetchURL is swapped for a fixture lookup, so
// nothing touches the network. To refresh a fixture, save the page/feed the
// scraper requests and gzip it under the same name.

import (
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chtitoon/tidetable/lescraper/core"
)

// fixtureNow is when the fixtures were captured.
var fixtureNow = time.Date(2026, 11, 21, 12, 0, 0, 0, newYork)

// useFixtures routes fetches to fixture files. A route key ending in "*"
// matches by prefix. Unrouted URLs fail the test. Returns a fetch log.
func useFixtures(t *testing.T, routes map[string]string) *[]string {
	t.Helper()
	oldFetch, oldNow := fetchURL, now
	t.Cleanup(func() { fetchURL, now = oldFetch, oldNow; resetSharedFeeds() })
	resetSharedFeeds()
	now = func() time.Time { return fixtureNow }
	var log []string
	fetchURL = func(url string) ([]byte, error) {
		log = append(log, url)
		name, ok := routes[url]
		if !ok {
			for k, v := range routes {
				if strings.HasSuffix(k, "*") && strings.HasPrefix(url, strings.TrimSuffix(k, "*")) {
					name, ok = v, true
				}
			}
		}
		if !ok {
			t.Errorf("unexpected fetch: %s", url)
			return nil, errors.New("no fixture for " + url)
		}
		if name == "" {
			return nil, errors.New("simulated failure")
		}
		return readFixture(t, name), nil
	}
	return &log
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name+".gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func scrape(t *testing.T, venue string) []core.Show {
	t.Helper()
	s, err := core.GetScraperForVenue(venue)
	if err != nil {
		t.Fatal(err)
	}
	shows, err := s.ScrapeShows()
	if err != nil {
		t.Fatalf("%s: %v", venue, err)
	}
	return shows
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, newYork) }

// findShow returns the first show matching title on date.
func findShow(t *testing.T, shows []core.Show, date time.Time, title string) core.Show {
	t.Helper()
	for _, s := range shows {
		if s.Title == title && s.Date.Equal(date) {
			return s
		}
	}
	t.Fatalf("no show %q on %s", title, date.Format("2006-01-02"))
	return core.Show{}
}

func countStatus(shows []core.Show) map[core.Status]int {
	m := map[core.Status]int{}
	for _, s := range shows {
		m[s.Status]++
	}
	return m
}

func checkShow(t *testing.T, got core.Show, want core.Show) {
	t.Helper()
	sameSupport := reflect.DeepEqual(got.Support, want.Support) || (len(got.Support) == 0 && len(want.Support) == 0)
	if got.Venue != want.Venue || got.Room != want.Room || !got.Date.Equal(want.Date) || got.Time != want.Time ||
		got.Status != want.Status || got.Headliner != want.Headliner || !sameSupport ||
		(want.URL != "" && got.URL != want.URL) || !got.OnSaleAt.Equal(want.OnSaleAt) {
		t.Errorf("show %q:\n got  %+v\n want %+v", want.Title, got, want)
	}
}

func TestMercuryLounge(t *testing.T) {
	routes := map[string]string{"https://mercuryeastpresents.com/mercurylounge/": "mercury_lounge_page1.html"}
	for n := 2; n <= 7; n++ {
		routes["https://mercuryeastpresents.com/mercurylounge/page/"+itoa(n)+"/"] = "mercury_lounge_page" + itoa(n) + ".html"
	}
	log := useFixtures(t, routes)
	shows := scrape(t, "Mercury Lounge")

	// 5 full pages of 20, 2 on page 6, page 7 empty (stop). The sidebar
	// widget's dateless .tw-section entries are not counted.
	if len(shows) != 102 {
		t.Errorf("got %d shows, want 102", len(shows))
	}
	if len(*log) != 7 {
		t.Errorf("fetched %d pages, want 7", len(*log))
	}

	checkShow(t, findShow(t, shows, day(2026, 11, 21), "aeter"), core.Show{
		Venue: "Mercury Lounge", Date: day(2026, 11, 21), Time: "18:00",
		Status:  core.StatusUnknown, // "Sales Ended"
		Support: []string{"Ja66"}, URL: "https://mercuryeastpresents.com/tm-event/aeter/",
	})
	// "*SOLD OUT*" is stripped from the title and sets the status.
	checkShow(t, findShow(t, shows, day(2026, 11, 21), "cortex"), core.Show{
		Venue: "Mercury Lounge", Date: day(2026, 11, 21), Time: "21:00", Status: core.StatusSoldOut,
	})
	checkShow(t, findShow(t, shows, day(2026, 11, 27), "Chemical X"), core.Show{
		Venue: "Mercury Lounge", Date: day(2026, 11, 27), Time: "21:00", Status: core.StatusOnSale,
		Support: []string{"Eviction Notice", "Deadly Skies", "Error 403"},
	})
	// Entities decode; dates in the new year parse with their own year.
	findShow(t, shows, day(2026, 11, 23), "Annalise & The Mother Underground")
	last := shows[len(shows)-1]
	if last.Date.Year() != 2027 || last.Date.Month() != time.May {
		t.Errorf("last show %s %q, want May 2027", last.Date.Format("2006-01-02"), last.Title)
	}
	for _, s := range shows {
		if strings.Contains(s.Title, "*") {
			t.Errorf("status marker left in title %q", s.Title)
		}
	}
}

func TestBoweryBallroomPage(t *testing.T) {
	shows, err := parseMercuryEastPage(readFixture(t, "bowery_ballroom_page1.html"), "Bowery Ballroom", "u")
	if err != nil {
		t.Fatal(err)
	}
	if len(shows) != 20 {
		t.Fatalf("got %d shows, want 20", len(shows))
	}
	if got := countStatus(shows); got[core.StatusOnSale] != 10 || got[core.StatusSoldOut] != 10 {
		t.Errorf("statuses %v, want 10 on sale, 10 sold out", got)
	}
	checkShow(t, findShow(t, shows, day(2026, 11, 22), "Father Of Peace – The Year Of Madness & The Mystery Century Tour"), core.Show{
		Venue: "Bowery Ballroom", Date: day(2026, 11, 22), Time: "19:00", Status: core.StatusSoldOut,
		Support: []string{"Roe Kapara"},
	})
	checkShow(t, findShow(t, shows, day(2026, 11, 28), "Stayofftherocks Fest"), core.Show{
		Venue: "Bowery Ballroom", Date: day(2026, 11, 28), Time: "17:00", Status: core.StatusOnSale,
		Support: []string{"Mila Culpa", "mer marcum", "Babe City", "Cooper Moore"},
	})
}

func TestParseClock(t *testing.T) {
	for in, want := range map[string]string{
		"Doors: 6:00 pm": "18:00", "7:00PM": "19:00", "12:30 pm": "12:30", "12:00 am": "00:00",
		"Doors: 11:15 a.m.": "11:15", "TBA": "",
	} {
		if got := parseClock(in); got != want {
			t.Errorf("parseClock(%q) = %q, want %q", in, got, want)
		}
	}
}

func aegRoutes() map[string]string {
	return map[string]string{
		boweryPresentsFeed: "aeg_events_59.json",
		"https://aegwebprod.blob.core.windows.net/json/events/85/events.json": "aeg_events_85.json",
	}
}

func TestBoweryPresentsVenues(t *testing.T) {
	log := useFixtures(t, aegRoutes())

	mhow := scrape(t, "Music Hall of Williamsburg")
	steel := scrape(t, "Brooklyn Steel")
	webster := scrape(t, "Webster Hall")
	nyc := scrape(t, "Bowery Presents NYC")

	// Feed 59 backs three venues but is downloaded once.
	if len(*log) != 2 {
		t.Errorf("fetched %v, want each feed once", *log)
	}
	for name, tc := range map[string]struct {
		shows []core.Show
		want  int
	}{
		"Music Hall of Williamsburg": {mhow, 10},
		"Brooklyn Steel":             {steel, 52},
		"Webster Hall":               {webster, 52},
		// 90 events in those rooms, minus 4 cancelled/postponed (including
		// both with a "TBD" date).
		"Bowery Presents NYC": {nyc, 86},
	} {
		if len(tc.shows) != tc.want {
			t.Errorf("%s: got %d shows, want %d", name, len(tc.shows), tc.want)
		}
	}

	// Linked headliner names, comma-separated support.
	checkShow(t, findShow(t, mhow, day(2026, 11, 22), "Jamila Woods: 10 Years of HEAVN"), core.Show{
		Venue: "Music Hall of Williamsburg", Date: day(2026, 11, 22), Time: "20:00", Status: core.StatusOnSale,
		Headliner: "Jamila Woods", Support: []string{"Mayyadda", "DJ Baby Jazz"},
		URL:      "https://www.axs.com/events/1554986/jamila-woods-10-years-of-heavn-tickets",
		OnSaleAt: time.Date(2026, 8, 14, 14, 0, 0, 0, time.UTC),
	})

	rooms := map[string]int{}
	for _, s := range nyc {
		rooms[s.Room]++
		if s.Venue != "Bowery Presents NYC" {
			t.Errorf("venue %q, want Bowery Presents NYC", s.Venue)
		}
	}
	for _, r := range []string{"Beacon Theatre", "Kings Theatre", "Terminal 5", "Forest Hills Stadium", "Radio City Music Hall",
		"Racket", "Knockdown Center", "Prospect Park Bandshell", "Madison Square Garden"} {
		if rooms[r] == 0 {
			t.Errorf("no shows in %s", r)
		}
	}
	if len(rooms) != 10 {
		t.Errorf("rooms %v: other regions leaked in?", rooms)
	}
	// "Coming Soon" with an on-sale date; a 00:00 start means no time yet.
	checkShow(t, findShow(t, nyc, day(2027, 2, 11), "WayV"), core.Show{
		Venue: "Bowery Presents NYC", Room: "Infosys Theater at Madison Square Garden", Date: day(2027, 2, 11),
		Time: "", Status: core.StatusComingSoon, Headliner: "WayV",
		OnSaleAt: time.Date(2026, 12, 4, 21, 0, 0, 0, time.UTC),
	})
}

func TestLPR(t *testing.T) {
	routes := map[string]string{"https://lpr.com/wp-json/wp/v2/lpr_events*": "lpr_rest_genres.json"}
	for n := 1; n <= 8; n++ {
		routes["https://lpr.com/wp-admin/admin-ajax.php?action=do_ajax&fn=get_homepageEvents&json%5Bdate%5D=20261121&paged="+itoa(n)] = "lpr_ajax_page" + itoa(n) + ".html"
	}
	useFixtures(t, routes)
	shows := scrape(t, "LPR")

	// 97 events: 11 outside NYC (Miami, California, ...), 13 DJ/dance
	// parties and 2 comedy nights dropped.
	if len(shows) != 73 {
		t.Errorf("got %d shows, want 73", len(shows))
	}
	checkShow(t, findShow(t, shows, day(2026, 12, 6), "Friendship & Charlotte Cornfield"), core.Show{
		Venue: "LPR", Room: "Main Space", Date: day(2026, 12, 6), Time: "18:30", Status: core.StatusUnknown,
		URL: "https://lpr.com/lpr_events/fcc26nyc/",
	})
	checkShow(t, findShow(t, shows, day(2026, 11, 21), "Catie Turner"), core.Show{
		Venue: "LPR", Room: "Stone Circle Theatre", Date: day(2026, 11, 21), Time: "19:00", Status: core.StatusUnknown,
	})
	for _, s := range shows {
		switch {
		case strings.Contains(s.Title, "Matinee Social Club"), strings.Contains(s.Title, "Emo Night"), s.Title == "Ana Gasteyer":
			t.Errorf("party/comedy kept: %q", s.Title)
		case s.Room == "Miami Beach Bandshell", s.Room == "Teragram Ballroom":
			t.Errorf("non-NYC room kept: %q in %s", s.Title, s.Room)
		}
	}
}

func TestLPRWithoutGenres(t *testing.T) {
	// If the REST API fails, parties are kept rather than failing the venue.
	routes := map[string]string{"https://lpr.com/wp-json/wp/v2/lpr_events*": ""}
	for n := 1; n <= 8; n++ {
		routes["https://lpr.com/wp-admin/admin-ajax.php?action=do_ajax&fn=get_homepageEvents&json%5Bdate%5D=20261121&paged="+itoa(n)] = "lpr_ajax_page" + itoa(n) + ".html"
	}
	useFixtures(t, routes)
	if shows := scrape(t, "LPR"); len(shows) != 86 {
		t.Errorf("got %d shows, want all 86 NYC shows", len(shows))
	}
}

func TestCapitolTheatre(t *testing.T) {
	useFixtures(t, map[string]string{
		"https://www.thecapitoltheatre.com/shows/rooms/the-capitol-theatre": "capitol_room_capitol.html",
		"https://www.thecapitoltheatre.com/shows/rooms/garcia-s":            "capitol_room_garcias.html",
	})
	shows := scrape(t, "The Capitol Theatre")

	// Capitol: 33 listed, minus 3 Nutcracker (Arts & Theatre/Dance) and
	// 1 comedy. Garcia's: 10. (The 3-day pass is dropped later by the
	// shared non-concert filter.)
	rooms := map[string]int{}
	for _, s := range shows {
		rooms[s.Room]++
	}
	if rooms["The Capitol Theatre"] != 29 || rooms["Garcia's"] != 10 || len(rooms) != 2 {
		t.Errorf("rooms %v, want 29 Capitol + 10 Garcia's", rooms)
	}
	for _, s := range shows {
		if strings.Contains(s.Title, "Nutcracker") || strings.Contains(s.Title, "Bridget Everett") {
			t.Errorf("non-music segment kept: %q", s.Title)
		}
	}
	checkShow(t, findShow(t, shows, day(2026, 11, 21), "Tom Hamilton"), core.Show{
		Venue: "The Capitol Theatre", Room: "Garcia's", Date: day(2026, 11, 21), Time: "20:00",
		Status:    core.StatusUnknown, // offsale
		Headliner: "Tom Hamilton", Support: []string{"Tasca"},
		URL:      "https://www.ticketmaster.com/tom-hamilton-port-chester-new-york-11-21-2026/event/1D00654DD35273D2",
		OnSaleAt: time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC),
	})
	checkShow(t, findShow(t, shows, day(2027, 4, 9), "Nathaniel Rateliff: Endless Horizon Tour"), core.Show{
		Venue: "The Capitol Theatre", Room: "The Capitol Theatre", Date: day(2027, 4, 9), Time: "20:00",
		Status: core.StatusOnSale, Headliner: "Nathaniel Rateliff", Support: []string{"Hannah Cohen"},
		OnSaleAt: time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC),
	})
	// Multi-day pass: start time 00:00:01 is a placeholder.
	if p := findShow(t, shows, day(2026, 11, 27), "Twiddle 3 DAY PASS"); p.Time != "" {
		t.Errorf("pass time %q, want empty", p.Time)
	}
}

func TestGrandCroton(t *testing.T) {
	useFixtures(t, map[string]string{grandFeed: "grand_happening.json"})
	shows := scrape(t, "The Grand")
	if len(shows) != 31 {
		t.Errorf("got %d shows, want 31", len(shows))
	}
	checkShow(t, findShow(t, shows, day(2026, 12, 8), "The LeeVees"), core.Show{
		Venue: "The Grand", Date: day(2026, 12, 8), Time: "19:00", Status: core.StatusUnknown,
		URL: "https://events.ticketleap.com/tickets/thegrandcroton/the-leevees",
	})
	checkShow(t, findShow(t, shows, day(2026, 11, 21), "Honest Dark & The Last Machine Operators"), core.Show{
		Venue: "The Grand", Date: day(2026, 11, 21), Time: "21:00", Status: core.StatusUnknown,
		URL: "https://thegrandcroton.com/happening/honest-dark",
	})
}

func TestTarrytown(t *testing.T) {
	useFixtures(t, map[string]string{
		"https://tickets.tarrytownmusichall.org/include/widgets/events/performancelist.asp?action=perf&listPageSize=100&listMaxSize=0&category=39&page=1": "tarrytown_music.json",
	})
	shows := scrape(t, "Tarrytown Music Hall")

	// 29 Music performances, minus 7 of "Annie The Musical" (two of them
	// school shows).
	if len(shows) != 22 {
		t.Errorf("got %d shows, want 22", len(shows))
	}
	checkShow(t, findShow(t, shows, day(2026, 11, 28), "Bill Kelly"), core.Show{
		Venue: "Tarrytown Music Hall", Room: "MH Studio", Date: day(2026, 11, 28), Time: "19:30",
		Status: core.StatusOnSale, URL: "https://tickets.tarrytownmusichall.org/eventperformances.asp?evt=3299",
		OnSaleAt: time.Date(2026, 11, 13, 17, 0, 0, 0, time.UTC), // noon Eastern
	})
	// Two performances of one event; the "(12/12/26)" suffix is dropped.
	n := 0
	for _, s := range shows {
		if s.Title == "Cherish the Ladies - A Celtic Christmas" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("got %d Cherish the Ladies performances, want 2", n)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
