# TideTable 🎸

> Every upcoming show at the venues I go to, in one file.

TideTable is a project I started over a gloomy weekend of October.

I initially thought about connecting shows of my favorite artists on Tidal with a google calendar. But after struggling with BandsInTown and Songkick APIs, I pivoted to something simpler: a weekly "Rolling Through" playlist of artists I'd probably never heard of, playing my venues that week.

**The playlist is retired.** TideTable now does two independent things, each a scheduled GitHub Actions job:

1. **[Show listings](#show-listings-datashowsjson)**: scrapes my [venues](venues.yaml) and commits every show in the next ~4 months to [`data/shows.json`](data/shows.json). No credentials needed.
2. **[Artist export](#daily-artist-export-artistsjson)**: exports my Tidal library as ranked artist lists, stored privately.

Matching one against the other happens elsewhere; neither job reads the other's data.

## Show listings (`data/shows.json`)

Every other day (and on demand from the Actions tab), `.github/workflows/daily-shows.yml` runs `go run .`, which scrapes every venue in [`venues.yaml`](venues.yaml) and writes `data/shows.json`:

```json
{
  "generated_at": "2026-11-21T11:23:41Z",
  "venues": [
    { "name": "Mercury Lounge", "url": "https://mercuryeastpresents.com/mercurylounge/", "ok": true, "shows": 92, "error": null }
  ],
  "shows": [
    {
      "date": "2026-12-06",
      "time": "18:30",
      "venue": "LPR",
      "room": "Main Space",
      "headliner": "Friendship",
      "support": ["Charlotte Cornfield"],
      "title": "Friendship & Charlotte Cornfield",
      "status": "unknown",
      "on_sale_at": null,
      "url": "https://lpr.com/lpr_events/fcc26nyc/"
    }
  ]
}
```

- **`venue`** is always one of the `venues[].name` entries. **`room`** names the hall when that venue covers several (LPR's rooms, Garcia's, the big Bowery Presents rooms), and is `null` otherwise.
- **`date` / `time`** are New York local time. `time` is the listed start; Mercury East only lists doors, so theirs is the doors time. It's `null` when the venue hasn't announced one.
- **`title`** is the listing text as published. **`headliner` / `support`** come from the venue's own lineup data when it has one (Bowery Presents, Webster Hall, Capitol, Mercury East's "with …" line), and otherwise from splitting the title (`lescraper/core/normalizer.go`). The split is best-effort.
- **`status`** is `on_sale`, `sold_out`, `coming_soon` or `unknown` (LPR and The Grand don't publish it). **`on_sale_at`** is the general on-sale time (UTC) where the venue publishes it.
- Cancelled and postponed shows are left out, and so are obvious non-concerts: DJ and dance nights, comedy, talks, musicals, ballet, films, dinners, parking and passes. A venue's own genre or category tags are used where it has them; otherwise a conservative title filter applies (`lescraper/core/filter.go`). When unsure, the show stays.
- Shows are sorted by date, then venue, then time, so diffs stay readable. The workflow commits only when something other than `generated_at` changed.

**Failures.** One venue failing (site down, markup changed) doesn't stop the others: it gets `"ok": false` with the error, and the workflow run shows a warning. A venue that returns 0 shows also counts as failed (`"error": "0 shows"`), since real venues always list something. The job fails, and the previous file stays as it was, only if every venue fails.

### Venues

| `venues.yaml` name | Listings | How it's read |
|---|---|---|
| Mercury Lounge | [mercuryeastpresents.com/mercurylounge](https://mercuryeastpresents.com/mercurylounge/) | Server-rendered list, `/page/N/` until an empty page |
| Bowery Ballroom | [mercuryeastpresents.com/boweryballroom](https://mercuryeastpresents.com/boweryballroom/) | Same platform as Mercury Lounge |
| Music Hall of Williamsburg | [bowerypresents.com/music-hall-of-williamsburg](https://www.bowerypresents.com/music-hall-of-williamsburg/) | The JSON feed the page loads (AEG `events/59`), filtered by venue ID |
| Brooklyn Steel | [bowerypresents.com/brooklyn-steel](https://www.bowerypresents.com/brooklyn-steel/) | Same feed |
| Webster Hall | [websterhall.com/shows](https://www.websterhall.com/shows/) | Its own JSON feed (AEG `events/85`) |
| Bowery Presents NYC | [bowerypresents.com/new-york-metro/calendar](https://www.bowerypresents.com/new-york-metro/calendar/) | Same feed as MHOW, keeping only Beacon Theatre, Kings Theatre, Terminal 5, Forest Hills Stadium, Radio City Music Hall, Racket, Knockdown Center, Prospect Park Bandshell, Madison Square Garden (and its Infosys Theater) |
| LPR | [lpr.com](https://lpr.com/) | The infinite-scroll endpoint behind the homepage (`admin-ajax.php`), NYC rooms only; genres from the WordPress REST API to drop DJ/comedy nights |
| The Capitol Theatre | [thecapitoltheatre.com/shows](https://www.thecapitoltheatre.com/shows) | Live Nation site: event data embedded in each room page (The Capitol Theatre, Garcia's) |
| The Grand | [thegrandcroton.com](https://thegrandcroton.com/) | Squarespace events page `/happening`, as JSON (`?format=json`) |
| Tarrytown Music Hall | [tickets.tarrytownmusichall.org](https://tickets.tarrytownmusichall.org/) | ShoWare box office JSON (`performancelist.asp`), Music category |

Everything is plain HTTP: either server-rendered HTML or the JSON a page loads. No headless browser. Each scraper's file in `lescraper/scrapers/` explains what it reads and why.

### Adding a venue

1. Look at the listings page. If the raw HTML (`curl -s URL`) contains the events, parse that. If it renders client-side, find the JSON it loads in the browser's network tab or the page's scripts. Check the source isn't truncated: look for pagination, "load more" endpoints and page-size caps.
2. Add a scraper in `lescraper/scrapers/<venue>.go` that implements `core.Scraper` (`GetVenues`, `GetName`, `GetURL`, `ScrapeShows`) and registers itself in `init()` with `core.RegisterVenue("<Name>", …)`. Fetch through `fetchURL` (or `fetchShared` for a feed other venues also read), so tests can serve fixtures. Return every upcoming show with `Venue`, `Date` (midnight New York time), and whatever of `Time`, `Room`, `Headliner`/`Support`, `Status`, `OnSaleAt` and `URL` the source has. The date window, dedupe and the shared non-concert filter are applied for you.
3. Save the page or feed it requests as `lescraper/scrapers/testdata/<name>.gz` and add a test to `scrapers_test.go` with `useFixtures`. Tests never hit the network.
4. Add the name to `venues.yaml`.

Platforms already handled, which a new venue might share: Mercury East's WordPress list (`MercuryEast`), AEG/Bowery Presents feeds (`BoweryPresents`: add an entry with the feed URL and venue IDs), and Live Nation venue sites (`CapitolTheatre`, rooms list). Elsewhere has a scraper (`elsewhere.go`) but isn't in `venues.yaml`.

### Running it locally

```bash
go run .                         # writes data/shows.json, prints a count per venue
go run . -out /tmp/shows.json -months 6
go test ./lescraper/... ./showexport
```

`VENUES_FILE` points at a different venue list.

## Tidal setup (for the artist export)

The artist export reads your Tidal library, so it needs Tidal API credentials. The show listings don't.

### 1. Get Tidal Developer Credentials

Register as a Tidal developer to get your API credentials:

1. Go to https://developer.tidal.com/
2. Create an account and register a new application
3. Set the **Redirect URI** to: `http://localhost:8080/callback`
4. Copy your **Client ID** and **Client Secret**

### 2. Add Client Credentials to GitHub Secrets

In your repository (or fork):

1. Go to **Settings** → **Secrets and variables** → **Actions**
2. Click **New repository secret** and add:
   - Name: `TIDAL_CLIENT_ID` / Value: `your_client_id`
   - Name: `TIDAL_CLIENT_SECRET` / Value: `your_client_secret`

### 3. Generate Tidal Access Tokens (One-Time Setup)

**This step must be done locally** because OAuth requires browser login.

Clone the repository:

```bash
git clone https://github.com/YOUR_USERNAME/tidetable.git
cd tidetable
go mod download
```

Run the authentication helper:

```bash
TIDAL_CLIENT_ID=your_client_id TIDAL_CLIENT_SECRET=your_client_secret go run cmd/auth/main.go
```

Your browser will open for Tidal login. After authorizing:
- Copy the **Access Token** from the terminal
- Copy the **Refresh Token** from the terminal

**You won't need to run this again.** Tokens refresh automatically.

### 4. Add Access Tokens to GitHub Secrets

Back in your repository's **Settings** → **Secrets and variables** → **Actions**, add two more secrets:

- Name: `TIDAL_ACCESS_TOKEN` / Value: `your_access_token`
- Name: `TIDAL_REFRESH_TOKEN` / Value: `your_refresh_token`

## Daily Artist Export (`artists.json`)

A second, independent job exports your Tidal library as two ranked lists of artists, for tools that match concert listings against what you actually listen to. It's independent of the show listings.

> **Your library is personal data, and forks of this repo are public.** By default the export is stored nowhere: it runs, validates, and is discarded. Point it at a private repo to keep it (see [Where the file lives](#where-the-file-lives)).

```json
{
  "generated_at": "2026-11-22T06:17:00Z",
  "my_artists":      [{ "tidal_id": "3566009", "name": "The National" }, …],
  "similar_artists": [{ "tidal_id": "…", "name": "Dry Cleaning" }, …]
}
```

Both lists are sorted most relevant first. Scores only decide the order and aren't stored in the file.

**`my_artists`.** Once a day, `.github/workflows/daily-artists.yml` runs `go run ./cmd/export-artists`, which reads your followed artists, liked albums and liked tracks (following every page) and scores each artist:

| Signal | Points |
|---|---|
| Followed | 3 |
| Each liked album where they are the main artist | 2 |
| Each liked track where they are the main artist | 1 |
| Anything above, liked or followed in the last 6 months | counts double |

Tidal's API has no listening history, so recent likes stand in for "listening to them a lot lately". Ties go to the artist you liked most recently. The main artist is the first credit Tidal lists; featured artists, compilations ("Various Artists"), label credits and playlist accounts don't score. Every artist with any score is listed.

**`similar_artists`.** For artists scoring 2 or more, the export fetches Tidal's similar artists (first page, up to 20). Those not already in your library are ranked by how many of your artists list them as similar, then by the total score of those artists.

### Where the file lives

Set the **`ARTISTS_DATA_REPO`** repository variable (Settings → Secrets and variables → Actions → **Variables**):

| `ARTISTS_DATA_REPO` | Behavior |
|---|---|
| *(unset, default)* | The export runs and is validated, then discarded. The run shows a warning. Nothing about your library leaves the runner. |
| `owner/name` | The file is committed to that repo, which should be **private**. Recommended. |
| `self` | The file is committed to `data/artists.json` in this repo, as before. If this repo is public, anyone can read your library. The run shows a warning. |

To set up a private data repo:

1. Create a private repo, e.g. `<you>/tidetable-data`, **with a README** so it has a default branch.
2. Create a [fine-grained personal access token](https://github.com/settings/personal-access-tokens/new) with **Repository access: only that repo** and **Contents: Read and write**. Nothing else.
3. In this repo, add the secret **`ARTISTS_DATA_TOKEN`** (the token) and the variable **`ARTISTS_DATA_REPO`** (`<you>/tidetable-data`).
4. Optionally set the variable **`ARTISTS_DATA_PATH`** for the file path inside that repo (default `artists.json`).

To read it with Claude, give Claude access to the private repo: in Claude Code, `gh` or a clone. On claude.ai, add the repo through the GitHub integration.

The workflow commits only when something other than `generated_at` changed. Sorting is deterministic, so diffs show real library changes. Because quiet stretches mean no commits to this repo, each run also re-enables its own workflow through the GitHub API, which stops GitHub disabling the schedule after 60 days of repo inactivity. Workflow logs in a public repo are public, so the export logs only counts, never artist names or IDs.

**Safety.** The file is built in memory, validated, then written atomically. If any API read fails after retries, or the artist count drops by more than half compared to the previous file, the run fails (GitHub emails you) and the previous file in the data repo stays as it was. Override a genuine drop locally with `-force`.

**Run it locally** with your secrets in 1Password (`.env` holds `op://` references for the four `TIDAL_*` variables):

```bash
op run --env-file=.env -- go run ./cmd/export-artists
```

This writes `data/artists.json`, which is gitignored. Pass `-out` to write elsewhere, e.g. into a local clone of your data repo.

**Re-authenticating.** The export refreshes the access token on its own, and Tidal's refresh token doesn't rotate, so the GitHub secrets don't need updating day to day. If a run fails with `token refresh failed`, the refresh token has expired or been revoked: run `go run cmd/auth/main.go` locally (step 3 of [Tidal setup](#tidal-setup-for-the-artist-export)), then update `TIDAL_ACCESS_TOKEN` and `TIDAL_REFRESH_TOKEN` in GitHub Secrets (and in 1Password), and re-run the workflow from the Actions tab.

## License

MIT

## Credits

Built with (some) love (and Claude Code).
