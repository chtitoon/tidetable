package scrapers

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"time"

	"github.com/chtitoon/tidetable/lescraper/core"
)

// GrandCroton scrapes The Grand (Croton-on-Hudson), a Squarespace site. Its
// events collection (/happening, linked from the homepage) is server-rendered;
// ?format=json returns the same page as structured data. "upcoming" holds
// every future event (the collection's pagination only pages through past ones).
type GrandCroton struct{}

const (
	grandURL  = "https://thegrandcroton.com/"
	grandFeed = "https://thegrandcroton.com/happening?format=json"
	grandBase = "https://thegrandcroton.com"
)

func init() {
	core.RegisterVenue("The Grand", &GrandCroton{})
}

func (s *GrandCroton) GetVenues() []string { return []string{"The Grand"} }
func (s *GrandCroton) GetName() string     { return "The Grand (Squarespace)" }
func (s *GrandCroton) GetURL() string      { return grandURL }

func (s *GrandCroton) ScrapeShows() ([]core.Show, error) {
	body, err := fetchURL(grandFeed)
	if err != nil {
		return nil, err
	}
	return parseGrandCroton(body)
}

var ticketLinkRe = regexp.MustCompile(`href="(https://events\.ticketleap\.com/tickets/[^"]+)"`)

func parseGrandCroton(body []byte) ([]core.Show, error) {
	var page struct {
		Upcoming *[]struct {
			Title     string `json:"title"`
			StartDate int64  `json:"startDate"` // epoch ms
			FullURL   string `json:"fullUrl"`
			Body      string `json:"body"`
		} `json:"upcoming"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("decoding events JSON: %w", err)
	}
	if page.Upcoming == nil {
		return nil, fmt.Errorf("no \"upcoming\" list; is /happening still an events collection?")
	}
	var shows []core.Show
	for _, e := range *page.Upcoming {
		start := time.UnixMilli(e.StartDate).In(newYork)
		url := grandBase + e.FullURL
		if m := ticketLinkRe.FindStringSubmatch(e.Body); m != nil {
			url = html.UnescapeString(m[1])
		}
		shows = append(shows, core.Show{
			Title:    cleanText(html.UnescapeString(e.Title)),
			Venue:    "The Grand",
			Date:     dateOnly(start),
			Time:     clock(start),
			Status:   core.StatusUnknown,
			URL:      url,
			VenueURL: grandURL,
		})
	}
	return shows, nil
}
