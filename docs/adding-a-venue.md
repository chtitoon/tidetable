# Adding a venue

Step-by-step instructions for adding a venue to the show listings, or fixing a venue whose scraper broke. Written for coding agents (any of them: Codex, Cursor, Claude Code, …) and just as usable by people.

Goal: a new venue shows up in `data/shows.json` with every upcoming show it lists, covered by a fixture-based test that never touches the network.

Inputs you need: the URL of the venue's site or listings page. If you're only given a URL, take the name from the site itself (its `og:site_name` or logo text, e.g. "Bearsville Theater"), drop any "The" only if the site does, and say which name you used in your report. Ask only if there's no URL, or the site covers several venues and it's unclear which one is meant. Everything else, find out yourself.

## 1. Check it isn't already handled

- `grep -rn 'RegisterVenue' lescraper/scrapers/` lists the scrapers, but platform scrapers register names from a list (`v.Name` in `mercury_east.go`, `bowery_presents.go`), so also `grep -rni '<venue name>' lescraper/scrapers/*.go`. A venue may be registered but missing from `venues.yaml` (Elsewhere is one): then just add it there and run it (step 6).
- Check whether the venue sits on a platform an existing scraper already reads. If it does, add an entry instead of writing a scraper:
  - **Mercury East Presents** (WordPress list, `/page/N/`): add to `mercuryEastVenues` in `mercury_east.go`.
  - **AEG / Bowery Presents** (the page loads `aegwebprod.blob.core.windows.net/json/events/<id>/events.json`): add to `boweryPresentsVenues` in `bowery_presents.go` with the feed URL and the venue ID(s) from the feed, mapping each ID to a room name, or `""` when the venue has one room.
  - **Live Nation venue site** (Next.js, event data in `self.__next_f.push(...)` chunks): see `capitol_theatre.go`.
  - **Squarespace events collection** (`?format=json` works): see `grand_croton.go`.
  - **ShoWare box office** (`performancelist.asp`): see `tarrytown.go`.
  - **WordPress with an infinite-scroll `admin-ajax.php` endpoint**: see `lpr.go`.
  - **WordPress with the Rockhouse/Etix "RHP Events" plugin** (`plugins/rhp-events/` in the page, Etix ticket links): events aren't in the REST API, and the `?post_type=rhp_events` RSS feed has publish dates, not show dates. Read the server-rendered list; see `bearsville_theater.go`. The markup around the list is the site theme's, so expect to adapt the selectors.

## 2. Find where the data really is

Use plain HTTP with the project's user agent. No headless browser: the project deliberately has none.

```bash
UA='tidetable/1.0 (+https://github.com/chtitoon/tidetable)'
curl -sL -A "$UA" "$URL" -o "$SCRATCH/page.html"; wc -c "$SCRATCH/page.html"
grep -c '<some show title you can see on the site>' "$SCRATCH/page.html"
```

Work through these in order and stop at the first that has the events:

1. **Server-rendered HTML.** The show titles are in the raw response.
2. **Structured data in the page:** `<script type="application/ld+json">` with `Event` objects, `__NEXT_DATA__`, `self.__next_f.push`, or inline `window.__INITIAL_STATE__`-style JSON.
3. **A JSON endpoint the page calls:** look in the page's scripts for `fetch(`, `ajax`, `.json`, `wp-json`, `admin-ajax.php`, `api/`. Common ones: WordPress REST (`/wp-json/wp/v2/...`), Squarespace (`?format=json`), ticketing platforms (Ticketmaster/Live Nation, AXS, Etix, DICE, Eventbrite, SeeTickets, ShoWare, Tixr). Prefer a JSON feed over HTML: it breaks less often.

Then **check the listing is complete**, since this is the most common bug:

- Is there pagination, "load more" or infinite scroll? Find the request behind it and follow it to the end.
- Does the feed take a page size or date range? Use the largest allowed, and follow pages until one comes back empty or short.
- Compare against the site: the last date you get should match the furthest show the site lists, often months out.
- If the feed states a total, check you got that many.
- Count each show once. Homepages often repeat upcoming shows in a slideshow or a "Featured" block above the real list. Find the container that holds only the main list and read cards inside it, and check the count against something independent (the site's RSS feed, a total, a count of distinct event links).

If the events can only be reached by running JavaScript, the site blocks plain requests (Cloudflare challenge, 403 with a browser UA too), or the terms clearly forbid it, stop and report what you found instead of working around it. That applies per source: a ticketing platform that blocks you (Etix answers `202` with an empty body, a bot check) just rules that source out, so fall back to the venue's own site if it has the listing.

## 3. Write the scraper

Create `lescraper/scrapers/<venue_snake_case>.go`. Start from the closest existing scraper; `grand_croton.go` is the smallest complete example. Shape:

```go
// <Type> scrapes <Venue> (<town>). Explain in a few lines what is read, why
// (e.g. "the page renders client-side from this feed"), and how the end of
// the listing is detected.
type <Type> struct{}

const <venue>URL = "https://…" // human-facing listings page

func init() { core.RegisterVenue("<Venue Name>", &<Type>{}) }

func (s *<Type>) GetVenues() []string { return []string{"<Venue Name>"} }
func (s *<Type>) GetName() string     { return "<Venue Name> (<platform>)" }
func (s *<Type>) GetURL() string      { return <venue>URL }

func (s *<Type>) ScrapeShows() ([]core.Show, error) {
	body, err := fetchURL(<feed or page URL>)
	if err != nil {
		return nil, err
	}
	return parse<Type>(body) // keep parsing separate from fetching, so tests can call it too
}
```

Rules the rest of the pipeline relies on:

- **Fetch only through `fetchURL`** (or `fetchShared` when several venues read the same feed). Never `http.Get`: tests swap `fetchURL` for fixtures, and it already retries and sets the user agent. Use the `now()` hook, never `time.Now()`, for any date parameter in a request.
- **Dates and times:** convert to `newYork` and use `dateOnly(t)` for `Date` and `clock(t)` for `Time` (`"19:30"`, 24h). Use the listed start time; if the source lists only doors, use doors and say so in the doc comment. Leave `Time` empty when unknown. Never invent a time.
- **Dates without a year** ("Dec 5"): first look for a source that has one, such as an event page's `ld+json` `startDate`. If the only usable listing is yearless and chronological, infer the year from `now()`: give the first card the year that puts it nearest today, then add a year each time the month goes backwards (`bearsville_theater.go`). Test the New Year rollover on a small synthetic page with `now` set in early January.
- **`Venue`** is the exact registered name. **`Room`** is set only when the venue has several halls; otherwise leave it empty.
- **`Title`** is the listing text as published, cleaned with `cleanText(html.UnescapeString(...))`. Set **`Headliner` / `Support`** only when the source gives the lineup explicitly (separate fields, a "with …" line). Otherwise leave them empty and the normalizer splits the title.
- **`Status`**: map the source's ticket state to `core.StatusOnSale`, `StatusSoldOut`, `StatusComingSoon` (on-sale date in the future), or `StatusUnknown` when the source doesn't say. Set **`OnSaleAt`** when the general on-sale time is published.
- **`URL`**: absolute link to the ticket or event page. **`VenueURL`**: the listings page.
- **Skip cancelled and postponed shows** in the scraper. If the source has genre or category tags, use them to drop non-concerts (DJ nights, comedy, talks, film, private events), as `lpr.go` does.
- **Don't** filter past dates, dedupe, skip placeholder names ("TBA", "@"), or match titles against the non-concert patterns ("2-Day Pass", "Comedy", "DJ Night"): `lescraper.go` applies `core.IsNonConcert` to every title and does the rest through `core`, for every venue. Call `core.IsNonConcert` yourself only on text other than the title, as `tarrytown.go` does with the performance name.
- **Bound pagination** with `maxPages`. On hitting it, return the shows so far with `errPageCap()`. If the listing is otherwise cut short (a feed shorter than its stated total), return the shows with an error wrapping `core.ErrIncomplete`. Both keep the shows and flag the venue.
- **Fail loudly when the structure changes.** If the expected container, key or JSON field is missing, return a descriptive error ("no \"upcoming\" list; is /happening still an events collection?") rather than an empty list.
- Use the HTML helpers in `htmlutil.go` (`parseHTML`, `findAll`, `byClass`, `textOf`, …) rather than adding a dependency.

## 4. Save fixtures

Save every URL the scraper requests, gzipped, under `lescraper/scrapers/testdata/`, named `<venue>_<what>[_pageN].<html|json>.gz`:

```bash
curl -sL -A "$UA" "$FEED_URL" | gzip -9 > lescraper/scrapers/testdata/<venue>_events.json.gz
```

Note the capture date. The existing fixtures were captured on 2026-11-21, which is `fixtureNow` in the tests.

## 5. Write the test

Add a test to `lescraper/scrapers/scrapers_test.go`, next to the others:

```go
func Test<Type>(t *testing.T) {
	useFixtures(t, map[string]string{<feed const>: "<venue>_events.json"}) // key ending in "*" matches by prefix
	shows := scrape(t, "<Venue Name>")
	if len(shows) != <N> {
		t.Errorf("got %d shows, want <N>", len(shows))
	}
	checkShow(t, findShow(t, shows, day(2027, 3, 14), "<exact title>"), core.Show{
		Venue: "<Venue Name>", Date: day(2027, 3, 14), Time: "20:00", Status: core.StatusOnSale,
		URL: "https://…",
	})
}
```

- Get `<N>` by counting the events in the fixture yourself (minus the cancelled ones and anything the scraper filters), not by copying what the scraper returns. Titles the shared non-concert filter will drop later still count here, since `scrape` calls the scraper directly.
- Check at least two shows field by field: a normal one, and one that's different (sold out, multiple acts, a second room, no time).
- If the scraper paginates, assert it stops at the end: check the fetch log `useFixtures` returns, and add a page-cap test like `TestMercuryEastPageCap` if the stop condition is unusual.
- If the scraper calls `now()` and your fixtures weren't captured on `fixtureNow`, set `now` to your capture date in that test.

## 6. Register and verify

1. Add the name to `venues.yaml`, with a short comment if it covers several rooms or filters anything.
2. Add a bullet for the venue under **Venues** in `README.md`: name, listings link, how it's read. The output-format bullets above it also name venues: add yours where it applies (`status` for venues that don't publish one, `headliner` / `support` for venues with their own lineup data, `time` if you used doors).
3. Run:

   ```bash
   gofmt -l . && go vet ./... && go test ./lescraper/... ./showexport
   VENUES_FILE=<scratch>/one.yaml go run . -out <scratch>/shows.json   # one.yaml lists just the new venue
   ```

   Always pass `-out` to a scratch path: `data/shows.json` is written by the scheduled workflow, so don't change it by hand or commit a local run.
4. Check the live output: the show count and the last date are plausible against the site, no obvious non-concerts got through, times and statuses look right, and `ok` is true. The run prints how many non-concerts it dropped; check they really aren't concerts. Also read `headliner` / `support`. If the title splitter mangles a title (e.g. "An Intimate Evening with Moe." becoming headliner "An Intimate Evening"), list it in your report rather than changing `core/normalizer.go`, which every venue shares.

## 7. Report back

Say which venue name you used, how many shows the live run found and through which date, which fields the source provides and which it doesn't (time, status, on-sale, lineup), what gets filtered and why, any titles the splitter gets wrong, and anything fragile (an undocumented endpoint, an HTML structure that could change, an inferred year).
