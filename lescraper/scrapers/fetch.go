package scrapers

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/chtitoon/tidetable/lescraper/core"
)

const userAgent = "tidetable/1.0 (+https://github.com/chtitoon/tidetable)"

// newYork is the time zone every venue lists in.
var newYork = func() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic("loading America/New_York: " + err.Error() + " (import time/tzdata)")
	}
	return loc
}()

// Hooks replaced in tests so no scraper makes live calls.
var (
	// fetchURL returns the body of a GET request.
	fetchURL = httpFetch
	// now is the current time, used for "upcoming" date parameters.
	now = time.Now
)

var httpClient = &http.Client{Timeout: 45 * time.Second}

// maxPages bounds how many pages a paginated scraper follows, so a broken
// end-of-list check or a "next" link that loops can't run forever. Hitting it
// keeps the shows found so far and reports the venue as incomplete.
const maxPages = 30

// errPageCap is returned (with the shows so far) when maxPages is reached.
func errPageCap() error {
	return fmt.Errorf("%w: still finding events after %d pages; stopped (pagination may be broken)", core.ErrIncomplete, maxPages)
}

// httpFetch GETs url, retrying network errors and 5xx/429 responses.
func httpFetch(url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*attempt) * 2 * time.Second)
		}
		body, retry, err := httpFetchOnce(url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return nil, lastErr
}

func httpFetchOnce(url string) (body []byte, retry bool, err error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("reading %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		retry = resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, retry, fmt.Errorf("fetching %s: HTTP %d", url, resp.StatusCode)
	}
	return body, false, nil
}

// sharedFeeds memoizes fetches of URLs that several venues read (the big
// Bowery Presents feed backs three of them), so each is downloaded once per run.
var sharedFeeds sync.Map // url -> *sharedFeed

type sharedFeed struct {
	once sync.Once
	body []byte
	err  error
}

func fetchShared(url string) ([]byte, error) {
	v, _ := sharedFeeds.LoadOrStore(url, &sharedFeed{})
	f := v.(*sharedFeed)
	f.once.Do(func() { f.body, f.err = fetchURL(url) })
	return f.body, f.err
}

// resetSharedFeeds clears the memo (tests).
func resetSharedFeeds() { sharedFeeds = sync.Map{} }

// dateOnly returns midnight New York time on t's New York calendar date.
func dateOnly(t time.Time) time.Time {
	t = t.In(newYork)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, newYork)
}

// clock formats t's New York wall time as "15:04".
func clock(t time.Time) string { return t.In(newYork).Format("15:04") }
