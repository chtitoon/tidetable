# TideTable 🎸

> Two always-fresh JSON files: every show at the venues I go to, and the artists I actually listen to.

TideTable is personal data plumbing for going to concerts. It doesn't recommend anything or have a UI. It keeps two clean, machine-readable files up to date on a schedule, for free, on GitHub Actions. Whatever reads them does the interesting part.

- **[`data/shows.json`](#show-listings-datashowsjson)**: every upcoming show at my [venues](venues.yaml), however far out. Scraped from the venue websites, no credentials needed, committed to this repo (public).
- **[`artists.json`](#artist-export-artistsjson)**: my Tidal library as ranked lists (all-time, lately, and similar artists I don't know yet). Read from the Tidal API with OAuth, stored in a private repo of your choice, or nowhere.

The two jobs never read each other's data. Matching happens downstream, for example:

- **"Who's worth seeing?"** Give an AI assistant (I use Claude) both files and ask what's coming up that I'd like, including artists I don't know yet but are similar to ones I do.
- **A "top artists" block on a personal site**, read straight from `my_artists` or `recent_artists`.
- A calendar feed, a weekly email, a playlist of who's in town: anything that can read JSON.

Splitting the data from what uses it is the point. Scrapers break and APIs change, so each job validates its output, keeps the last good file when it fails, and commits only real changes, so the git history doubles as a changelog of announcements and of your taste.

### How it started

I started TideTable over a gloomy October weekend, wanting the shows of my favorite Tidal artists in my calendar. After struggling with the Bandsintown and Songkick APIs, I settled on scraping venues myself and generating a weekly "Rolling Through" playlist of artists playing my venues that week. The playlist is retired: the data turned out to be the useful part, and anything that consumes it can be built elsewhere.

## Make it yours

TideTable is built around one person's venues and one streaming service, but nothing about the shape is specific to New York or Tidal. Fork it and keep the parts you want.

**1. Fork and turn on Actions.** GitHub disables workflows in a fork: open the **Actions** tab and enable them. *Shows Export* then runs every other day and can be triggered by hand (**Run workflow**).

**2. Point it at your venues.**

- Edit [`venues.yaml`](venues.yaml) to list your venues, then ask your coding agent to add each one that isn't supported yet: see [Adding a venue](#adding-a-venue).
- Delete the scrapers you don't need, or leave them: only the venues in `venues.yaml` are scraped.

**3. Set your time zone.** Dates and times are local to the venues, hard-coded as `America/New_York` in [`main.go`](main.go) and [`lescraper/scrapers/fetch.go`](lescraper/scrapers/fetch.go). Change both if you're elsewhere. Also update the `userAgent` in `fetch.go` to point at your fork, so venues know who's calling.

**4. Decide about your listening data.**

- **Skip it.** The show listings work alone. Disable *Daily Artists Export* in the Actions tab (it fails without Tidal secrets) or delete `.github/workflows/daily-artists.yml`.
- **Tidal.** Follow [Tidal setup](#tidal-setup), then point the export at a private repo ([Where the file lives](#where-the-file-lives)). By default it stores nothing, so a public fork never publishes your library by accident.
- **Another service** (Spotify, Apple Music, Last.fm, …). The scoring in `artistexport` works on followed artists, liked albums and liked tracks with the date each was added, plus similar artists and a popularity score. It reads them as `tidal` package types today, so write a client for your service that fills those in (or generalize the types), keep the `artists.json` format, and everything downstream keeps working. Last.fm would also give you real listening history, which Tidal's API doesn't have.

**5. Use the files.** `data/shows.json` is public at `https://raw.githubusercontent.com/<you>/tidetable/main/data/shows.json`. `artists.json` sits in your private repo; give your assistant or site read access to it. Both formats are documented below.

What's deliberately left out: no database, server, headless browser or paid service. Everything is plain HTTP and JSON committed to git, so a fork costs nothing to run and is easy to debug from the commit history.

## Show listings (`data/shows.json`)

Every other day (and on demand from the Actions tab), `.github/workflows/daily-shows.yml` runs `go run .`, which scrapes every venue in [`venues.yaml`](venues.yaml) and writes `data/shows.json`:

```json
{
  "generated_at": "2026-11-21T11:23:41Z",
  "venues": [
    { "name": "Mercury Lounge", "url": "https://mercuryeastpresents.com/mercurylounge/", "ok": true, "shows": 102, "skipped": 0, "latest_date": "2027-05-07", "error": null }
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

- **`shows`** holds every show the venue lists from today on, with no cut-off date: big shows are often announced a year out. Each venue's **`latest_date`** is its last show's date, i.e. how far its listing reaches (`null` with no shows). Scrapers follow pagination to the end of each listing, up to 30 pages per venue.
- **`venue`** is always one of the `venues[].name` entries. **`room`** names the hall when that venue covers several (LPR's rooms, Garcia's, the big Bowery Presents rooms), and is `null` otherwise.
- **`date` / `time`** are New York local time. `time` is the listed start; Mercury East only lists doors, so theirs is the doors time. It's `null` when the venue hasn't announced one.
- **`title`** is the listing text as published. **`headliner` / `support`** come from the venue's own lineup data when it has one (Bowery Presents, Webster Hall, Capitol, Mercury East's "with …" line), and otherwise from splitting the title (`lescraper/core/normalizer.go`). The split is best-effort.
- **`status`** is `on_sale`, `sold_out`, `coming_soon` or `unknown` (LPR and The Grand don't publish it). **`on_sale_at`** is the general on-sale time (UTC) where the venue publishes it.
- Cancelled and postponed shows are left out, and so are obvious non-concerts: DJ and dance nights, comedy, talks, musicals, ballet, films, dinners, parking and passes. A venue's own genre or category tags are used where it has them; otherwise a conservative title filter applies (`lescraper/core/filter.go`). When unsure, the show stays.
- Shows whose headliner is a placeholder are skipped and counted in the venue's **`skipped`**: fewer than 3 letters or digits once punctuation and spaces are stripped (Bowery Ballroom has listed shows named just `@`), or "TBA", "TBD", "To be announced", "Special guest(s)". Placeholder entries in `support` are dropped and the show kept.
- Shows are sorted by date, then venue, then time, so diffs stay readable. The workflow commits only when something other than `generated_at` changed.

**Failures.** One venue failing (site down, markup changed) doesn't stop the others: it gets `"ok": false` with the error, and the workflow run shows a warning. A venue that returns 0 shows also counts as failed (`"error": "0 shows"`), since real venues always list something. So does a listing that couldn't be read to the end (page cap reached, a feed shorter than its stated total): its shows are kept, but the venue gets `"ok": false` and an error saying where it stopped. The job fails, and the previous file stays as it was, only if every venue fails.

### Venues

Each entry is the name used in `venues.yaml`, its listings page, and how it's read.

- **Mercury Lounge** ([mercuryeastpresents.com/mercurylounge](https://mercuryeastpresents.com/mercurylounge/)): Server-rendered list, `/page/N/` until an empty page.
- **Bowery Ballroom** ([mercuryeastpresents.com/boweryballroom](https://mercuryeastpresents.com/boweryballroom/)): Same platform as Mercury Lounge.
- **Music Hall of Williamsburg** ([bowerypresents.com/music-hall-of-williamsburg](https://www.bowerypresents.com/music-hall-of-williamsburg/)): The JSON feed the page loads (AEG `events/59`), filtered by venue ID. "Load More" only reveals more of that feed, so one fetch has every show.
- **Brooklyn Steel** ([bowerypresents.com/brooklyn-steel](https://www.bowerypresents.com/brooklyn-steel/)): Same feed.
- **Webster Hall** ([websterhall.com/shows](https://www.websterhall.com/shows/)): Its own JSON feed (AEG `events/85`).
- **Bowery Presents NYC** ([bowerypresents.com/new-york-metro/calendar](https://www.bowerypresents.com/new-york-metro/calendar/)): Same feed as MHOW, keeping only Beacon Theatre, Kings Theatre, Terminal 5, Forest Hills Stadium, Radio City Music Hall, Racket, Knockdown Center, Prospect Park Bandshell, Madison Square Garden (and its Infosys Theater).
- **LPR** ([lpr.com](https://lpr.com/)): The infinite-scroll endpoint behind the homepage (`admin-ajax.php`), NYC rooms only; genres from the WordPress REST API to drop DJ/comedy nights.
- **The Capitol Theatre** ([thecapitoltheatre.com/shows](https://www.thecapitoltheatre.com/shows)): Live Nation site, with event data embedded in each room page (The Capitol Theatre, Garcia's).
- **The Grand** ([thegrandcroton.com](https://thegrandcroton.com/)): Squarespace events page `/happening`, as JSON (`?format=json`).
- **Tarrytown Music Hall** ([tickets.tarrytownmusichall.org](https://tickets.tarrytownmusichall.org/)): ShoWare box office JSON (`performancelist.asp`), Music category.

Everything is plain HTTP: either server-rendered HTML or the JSON a page loads. No headless browser. Each scraper's file in `lescraper/scrapers/` explains what it reads and why.

### Adding a venue

Ask your coding agent (Codex, Cursor, Claude Code, …) to "add <venue>, listings at <url>". [`AGENTS.md`](AGENTS.md) sends it to [`docs/adding-a-venue.md`](docs/adding-a-venue.md), which covers the whole process, from finding where the data is to checking the scraper on the live site.

### Running it locally

```bash
go run .                         # writes data/shows.json, prints a count and latest date per venue
go run . -out /tmp/shows.json
go test ./lescraper/... ./showexport
```

`VENUES_FILE` points at a different venue list.

## Tidal setup

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

## Artist export (`artists.json`)

A second, independent job exports your Tidal library as ranked lists of artists, for tools that match concert listings against what you actually listen to, or a site showing your top artists.

> **Your library is personal data, and forks of this repo are public.** By default the export is stored nowhere: it runs, validates, and is discarded. Point it at a private repo to keep it (see [Where the file lives](#where-the-file-lives)).

```json
{
  "generated_at": "2026-11-22T06:17:00Z",
  "my_artists":      [{ "tidal_id": "3566009", "name": "The National" }, …],
  "recent_artists":  [{ "tidal_id": "…", "name": "Big Thief" }, …],
  "similar_artists": [{ "tidal_id": "…", "name": "Dry Cleaning" }, …]
}
```

All lists are sorted most relevant first. Scores only decide the order and aren't stored in the file.

Once a day, `.github/workflows/daily-artists.yml` runs `go run ./cmd/export-artists`, which reads your followed artists, liked albums and liked tracks (following every page) and scores each artist. Only the main artist of an album or track scores: the first credit Tidal lists. Featured artists, compilations ("Various Artists"), label credits and playlist accounts don't. Tidal profiles with the same name (Tidal has duplicates) are merged into one artist.

```
score = follow + 3 × ln(1 + albums) + 1.5 × ln(1 + tracks)
```

`albums` and `tracks` count your likes, weighted by age in `recent_artists` only. The logarithm keeps a pile of hearted tracks from outweighing a follow and a couple of albums.

**`my_artists`** is the all-time ranking of every artist with any score. A follow is worth 4, and every like counts in full, however old: recency only breaks ties. So the list changes only when your library does.

**`recent_artists`** is what you're into lately: only follows and likes from the last 90 days count, fading with a 45-day half-life. An artist needs at least 2 of them to be listed, so a single heart doesn't top the list. It can be short or empty after a quiet stretch. Every recent artist is also in `my_artists`.

Tidal's API has no listening history, so recent likes stand in for "listening to them a lot lately". In both lists, ties go to the artist you liked most recently.

**`similar_artists`.** For artists with an all-time score of 3 or more (a follow, or an album and a track), the export fetches Tidal's similar artists (first page, up to 20). Those not already in your library are ranked by the summed all-time score of your artists that list them, scaled down by Tidal popularity (a fully popular artist keeps a quarter), so lesser-known artists come first.

### Where the file lives

Set the **`ARTISTS_DATA_REPO`** repository variable (Settings → Secrets and variables → Actions → **Variables**):

- **Unset (default):** the export runs and is validated, then discarded. The run shows a warning. Nothing about your library leaves the runner.
- **`owner/name`:** the file is committed to that repo, which should be **private**. Recommended.
- **`self`:** the file is committed to `data/artists.json` in this repo, as before. If this repo is public, anyone can read your library. The run shows a warning.

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

**Re-authenticating.** The export refreshes the access token on its own, and Tidal's refresh token doesn't rotate, so the GitHub secrets don't need updating day to day. If a run fails with `token refresh failed`, the refresh token has expired or been revoked: run `go run cmd/auth/main.go` locally (step 3 of [Tidal setup](#tidal-setup)), then update `TIDAL_ACCESS_TOKEN` and `TIDAL_REFRESH_TOKEN` in GitHub Secrets (and in 1Password), and re-run the workflow from the Actions tab.

## License

MIT

## Credits

Built with (some) love (and Claude Code).
