# TideTable 🎸

> Discover new artists playing near you week in and week out.

TideTable is a project I started over a gloomy weekend of October.

I initially thought about connecting shows of my favorite artists on Tidal with a google calendar. But after struggling with BandsInTown and Songkick APIs, I pivoted to something simpler: create a weekly playlist from artists I've probably never heard of. That way, eventually—maybe, really maybe, if I'm in the mood and have an empty agenda—I'll go to one of them. That's it. That's the whole idea.

<img width="290" alt="image" src="https://github.com/user-attachments/assets/ae5f22b5-023d-4ef4-8d7b-ef3790143525" />



## How It Works

1. Every Friday, using github actions, it automatically checks your favorite [venues](venues.yaml) for upcoming shows (Monday-Sunday of the following week)
2. Grabs every artist name from the lineups
3. Searches Tidal, picks a top track from each artist, bundles them into a weekly "Rolling Through" playlist 

## Setup

This is a GitHub Actions-first setup. Everything runs automatically once configured.

**Prerequisites:**
- A Tidal account (HiFi or HiFi Plus)
- A GitHub account
- Go 1.21+ installed (only needed for one-time token generation)

### 1. Fork This Repository

Click the "Fork" button at the top of this page to create your own copy.

### 2. Get Tidal Developer Credentials

Register as a Tidal developer to get your API credentials:

1. Go to https://developer.tidal.com/
2. Create an account and register a new application
3. Set the **Redirect URI** to: `http://localhost:8080/callback`
4. Copy your **Client ID** and **Client Secret**

### 3. Add Client Credentials to GitHub Secrets

In your forked repository:

1. Go to **Settings** → **Secrets and variables** → **Actions**
2. Click **New repository secret** and add:
   - Name: `TIDAL_CLIENT_ID` / Value: `your_client_id`
   - Name: `TIDAL_CLIENT_SECRET` / Value: `your_client_secret`

### 4. Generate Tidal Access Tokens (One-Time Setup)

**This step must be done locally** because OAuth requires browser login.

Clone your forked repository:

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

### 5. Add Access Tokens to GitHub Secrets

Back in your repository's **Settings** → **Secrets and variables** → **Actions**, add two more secrets:

- Name: `TIDAL_ACCESS_TOKEN` / Value: `your_access_token`
- Name: `TIDAL_REFRESH_TOKEN` / Value: `your_refresh_token`

### 6. Configure Your Venues

Edit `venues.yaml` to add your go-to spots:

```yaml
venues:
  - name: "Mercury Lounge / Bowery Ballroom"
    url: "https://mercuryeastpresents.com/calendar-mr/"
    location: "New York, NY"
    scraper_type: "mercury_east"

  - name: "Your Favorite Venue"
    url: "https://venuename.com/shows"
    location: "Brooklyn, NY"
    scraper_type: "generic"
```

**Supported Scraper Types:**
- `mercury_east`: Works for Mercury East Presents venues (Mercury Lounge, Bowery Ballroom, etc.)

Want your local venue supported? PRs welcome or open an issue.

Commit and push your `venues.yaml` changes to your fork:

```bash
git add venues.yaml
git commit -m "Configure my favorite venues"
git push
```

### 7. Test the Workflow

In your GitHub repository:

1. Go to the **Actions** tab
2. Select **Weekly Playlist Update**
3. Click **Run workflow** → **Run workflow**

Watch it run! In about a minute, check your Tidal account for the new playlist.

### 8. Done!
That's it. Every Friday at 9 AM UTC (5 AM EST), your playlist updates automatically with fresh artists from the upcoming week's shows.

No maintenance required. Just listen, and discover.

## Daily Artist Export (`artists.json`)

A second, independent job exports your Tidal library as two ranked lists of artists, for tools that match concert listings against what you actually listen to. It doesn't touch the playlist feature.

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

**Re-authenticating.** The export refreshes the access token on its own, and Tidal's refresh token doesn't rotate, so the GitHub secrets don't need updating day to day. If a run fails with `token refresh failed`, the refresh token has expired or been revoked: run `go run cmd/auth/main.go` locally (step 4 above), then update `TIDAL_ACCESS_TOKEN` and `TIDAL_REFRESH_TOKEN` in GitHub Secrets (and in 1Password), and re-run the workflow from the Actions tab.

## License

MIT

## Credits

Built with (some) love (and Claude Code).
