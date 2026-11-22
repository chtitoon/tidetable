package scrapers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/chtitoon/tidetable/lescraper/core"
)

// LPR scrapes lpr.com, which books its own Main Space plus other rooms in
// NYC and elsewhere. The homepage shows ~3 weeks; the rest comes from the
// infinite-scroll endpoint it calls (admin-ajax.php, fn=get_homepageEvents,
// 14 events per page, empty past the end). Genres aren't in that HTML, so
// they're read from the WordPress REST API by post ID to drop DJ/dance
// parties and comedy, which LPR tags as such.
type LPR struct{}

const (
	lprURL      = "https://lpr.com/"
	lprAjax     = "https://lpr.com/wp-admin/admin-ajax.php?action=do_ajax&fn=get_homepageEvents&json%%5Bdate%%5D=%s&paged=%d"
	lprRest     = "https://lpr.com/wp-json/wp/v2/lpr_events?per_page=100&_fields=id,class_list&include="
	lprMaxPages = 40
)

// lprNYCRooms are the NYC rooms LPR lists (matched case-insensitively).
// Shows in other rooms are skipped and their room names logged, so a new NYC
// room shows up in the run log and can be added here.
var lprNYCRooms = []string{
	"Main Space",
	"Stone Circle Theatre",
	"First Unitarian Church",
	"Holy Trinity Lutheran Church",
	"Knockdown Center",
	"Littlefield",
	"National Sawdust",
	"Stern Auditorium / Perelman Stage at Carnegie Hall",
	"The Sultan Room",
	"The Town Hall",
}

// lprSkipGenres are genre tags (event_genre-*) that mark non-concerts.
var lprSkipGenres = map[string]bool{"dj": true, "dance-party": true, "party": true}

func init() {
	core.RegisterVenue("LPR", &LPR{})
}

func (s *LPR) GetVenues() []string { return []string{"LPR"} }
func (s *LPR) GetName() string     { return "LPR" }
func (s *LPR) GetURL() string      { return lprURL }

func (s *LPR) ScrapeShows() ([]core.Show, error) {
	from := now().In(newYork).Format("20060102")
	var all []lprEvent
	seen := map[string]bool{}
	for page := 1; ; page++ {
		if page > lprMaxPages {
			return nil, fmt.Errorf("still finding events after %d pages; end-of-list check may be broken", lprMaxPages)
		}
		body, err := fetchURL(fmt.Sprintf(lprAjax, from, page))
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		events, err := parseLPRPage(body)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		if len(events) == 0 {
			break
		}
		for _, e := range events {
			if !seen[e.postID+e.show.URL] {
				seen[e.postID+e.show.URL] = true
				all = append(all, e)
			}
		}
	}

	genres, err := fetchLPRGenres(all)
	if err != nil {
		// Genres only filter out parties; keep everything rather than fail.
		fmt.Printf("    Warning: LPR genres unavailable, not filtering DJ/comedy nights: %v\n", err)
	}
	return filterLPR(all, genres), nil
}

type lprEvent struct {
	postID string
	show   core.Show
}

var lprPostIDRe = regexp.MustCompile(`^event_(\d+)$`)

// parseLPRPage reads one page of event cards (div.event[dateOfEvent]).
func parseLPRPage(body []byte) ([]lprEvent, error) {
	doc, err := parseHTML(body)
	if err != nil {
		return nil, err
	}
	var out []lprEvent
	cards := findAll(doc, func(n *htmlNode) bool { return hasClass(n, "event") && attr(n, "dateofevent") != "" })
	for _, card := range cards {
		day := attr(card, "dateofevent")
		date, err := time.ParseInLocation("20060102", day, newYork)
		if err != nil {
			return nil, fmt.Errorf("parsing dateOfEvent %q: %w", day, err)
		}
		var postID string
		for _, c := range strings.Fields(attr(card, "class")) {
			if m := lprPostIDRe.FindStringSubmatch(c); m != nil {
				postID = m[1]
			}
		}
		link := ""
		if a := find(card, byClass("eventSingleLink")); a != nil {
			link = attr(a, "href")
		}
		out = append(out, lprEvent{
			postID: postID,
			show: core.Show{
				Title:    textOf(card, "black_visible"),
				Venue:    "LPR",
				Room:     textOf(card, "spaceTitle"),
				Date:     date,
				Time:     parseClock(textOf(card, "time")),
				Status:   core.StatusUnknown, // the listing has no ticket status
				URL:      link,
				VenueURL: lprURL,
			},
		})
	}
	return out, nil
}

// fetchLPRGenres returns post ID -> genre slugs, from the REST API's class_list.
func fetchLPRGenres(events []lprEvent) (map[string][]string, error) {
	var ids []string
	for _, e := range events {
		if e.postID != "" {
			ids = append(ids, e.postID)
		}
	}
	genres := map[string][]string{}
	for start := 0; start < len(ids); start += 100 {
		end := min(start+100, len(ids))
		body, err := fetchURL(lprRest + url.QueryEscape(strings.Join(ids[start:end], ",")))
		if err != nil {
			return nil, err
		}
		chunk, err := parseLPRGenres(body)
		if err != nil {
			return nil, err
		}
		for k, v := range chunk {
			genres[k] = v
		}
	}
	return genres, nil
}

func parseLPRGenres(body []byte) (map[string][]string, error) {
	var posts []struct {
		ID        int      `json:"id"`
		ClassList []string `json:"class_list"`
	}
	if err := json.Unmarshal(body, &posts); err != nil {
		return nil, fmt.Errorf("decoding REST genres: %w", err)
	}
	out := map[string][]string{}
	for _, p := range posts {
		var g []string
		for _, c := range p.ClassList {
			if rest, ok := strings.CutPrefix(c, "event_genre-"); ok {
				g = append(g, rest)
			}
		}
		out[fmt.Sprint(p.ID)] = g
	}
	return out, nil
}

func filterLPR(events []lprEvent, genres map[string][]string) []core.Show {
	nyc := map[string]bool{}
	for _, r := range lprNYCRooms {
		nyc[roomKey(r)] = true
	}
	var shows []core.Show
	skippedRooms := map[string]int{}
	for _, e := range events {
		if !nyc[roomKey(e.show.Room)] {
			skippedRooms[e.show.Room]++
			continue
		}
		if lprIsParty(genres[e.postID]) {
			continue
		}
		shows = append(shows, e.show)
	}
	if len(skippedRooms) > 0 {
		var names []string
		for r, n := range skippedRooms {
			names = append(names, fmt.Sprintf("%s (%d)", r, n))
		}
		sort.Strings(names)
		fmt.Printf("    LPR: skipped rooms not on the NYC list: %s\n", strings.Join(names, ", "))
	}
	return shows
}

func lprIsParty(genres []string) bool {
	for _, g := range genres {
		if lprSkipGenres[g] || strings.HasPrefix(g, "comedy") {
			return true
		}
	}
	return false
}

// roomKey normalizes a room name for matching ("Pappy &amp; Harriet’s" etc.).
func roomKey(s string) string {
	s = strings.NewReplacer("’", "'", "‘", "'").Replace(strings.ToLower(cleanText(s)))
	return s
}
