# TideTable 🎵

> **Discover live music differently.** Stop scrolling through the same old playlists. Start with the venues you love.

## The Problem

You know that feeling? You're listening to the same artists on repeat. Ticket prices for big-name shows are insane. Meanwhile, incredible music is happening every week at your favorite local venues—artists you've never heard of, genres you didn't know you'd love.

What if you could discover new music based on who's actually playing near you, instead of what Spotify's algorithm thinks you like?

## The Solution

**TideTable flips music discovery on its head.** Instead of starting with artists you already know, it starts with venues you love.

Every week, TideTable:
1. Checks what shows are coming up at your favorite venues
2. Finds each artist on Tidal
3. Creates a fresh playlist with a taste of everyone performing

Now you have the weekend to listen, vibe, and decide: "Wait, who is this? I need to see them live."

Suddenly you're catching amazing shows for $15-25, discovering artists before they blow up, and your music library is actually growing instead of calcifying around the same dozen bands.

## Why This Rocks

- **Escape the echo chamber**: Your music taste shouldn't be a closed loop
- **Affordable live music**: Local shows = reasonable prices (and better sound anyway)
- **Support rising artists**: Catch them in small venues before they're playing arenas
- **Zero effort**: Set it up once, get fresh music every Friday
- **Perfect timing**: Updated Fridays so you have the weekend to listen and plan

## How It Works

1. **Venue Scraping**: Automatically checks your favorite venues for upcoming shows (Monday-Sunday of the following week)
2. **Artist Discovery**: Grabs every artist name from the lineups
3. **Playlist Magic**: Searches Tidal, picks a top track from each artist, bundles them into a weekly playlist
4. **Friday Vibes**: GitHub Actions runs every Friday morning—your playlist is ready before your coffee

The best part? It's all automatic. You just wake up, hit play, and discover.

## Setup

### Quick Start

**Prerequisites:**
- Go 1.21+ installed
- A Tidal account (HiFi or HiFi Plus)
- A GitHub account for auto-updates
- 1Password CLI (optional but recommended for secret management)

### 1. Clone and Install

```bash
git clone https://github.com/chtitoon/tidetable.git
cd tidetable
go mod download
```

### 2. Get Tidal Developer Access

First, you need to register as a Tidal developer:

1. Go to https://developer.tidal.com/
2. Create an account and register a new app
3. Set the **Redirect URI** to: `http://localhost:8080/callback`
4. Grab your Client ID and Client Secret

Now authenticate:

```bash
TIDAL_CLIENT_ID=your_client_id TIDAL_CLIENT_SECRET=your_client_secret go run cmd/auth/main.go
```

Your browser will open, you'll log into Tidal, authorize the app, and boom—your access tokens appear in the terminal. Save these for the next step.

### 3. Configure Your Venues

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
- `generic`: Basic HTML scraper (may need customization)

Want your local venue supported? PRs welcome or open an issue.

### 4. Set Up Environment Variables

Create a `.env` file:

```env
TIDAL_CLIENT_ID=your_tidal_client_id
TIDAL_CLIENT_SECRET=your_tidal_client_secret
TIDAL_ACCESS_TOKEN=your_tidal_access_token
TIDAL_REFRESH_TOKEN=your_tidal_refresh_token
PLAYLIST_NAME="Rolling Through"
VENUES_FILE="venues.yaml"
```

**Using 1Password (Recommended):**

Store your secrets in 1Password and reference them:

```env
TIDAL_CLIENT_ID="op://Private/Tidal/tidetable/clientId"
TIDAL_CLIENT_SECRET="op://Private/Tidal/tidetable/clientSecret"
TIDAL_ACCESS_TOKEN="op://Private/Tidal/tidetable/accessToken"
TIDAL_REFRESH_TOKEN="op://Private/Tidal/tidetable/refreshToken"
```

Then run with:
```bash
op run --env-file=.env -- go run main.go
```

Or just use the helper script:
```bash
./run.sh
```

### 5. Test It Out

```bash
./run.sh
```

You'll see it scraping venues, finding artists, building your playlist. In a minute or two, check Tidal—your playlist is live.

### 6. Automate with GitHub Actions

Set it and forget it:

1. Add these secrets to your GitHub repo (Settings → Secrets and variables → Actions):
   - `TIDAL_CLIENT_ID`
   - `TIDAL_CLIENT_SECRET`
   - `TIDAL_ACCESS_TOKEN`
   - `TIDAL_REFRESH_TOKEN`

2. The workflow runs automatically every Friday at 9 AM UTC (5 AM EST)

3. Want to run it manually? Go to Actions tab → Weekly Playlist Update → Run workflow

Done. Every Friday, fresh playlist. Every weekend, new discoveries.

## Customization

### Change the Schedule

Edit `.github/workflows/weekly-playlist.yml`:

```yaml
schedule:
  - cron: '0 9 * * 5'  # Every Friday at 9 AM UTC
```

### Change the Playlist Name

Set `PLAYLIST_NAME` in your `.env` file or GitHub secrets:

```env
PLAYLIST_NAME="Coming Up This Week"
```

Default is "Rolling Through" if you don't set it.

### Add More Venues

Just edit `venues.yaml` and add as many as you want. The more venues, the more variety.

## Project Structure

```
tidetable/
├── .github/workflows/
│   └── weekly-playlist.yml  # Auto-runs every Friday
├── cmd/auth/
│   └── main.go              # Tidal OAuth helper
├── config/
│   └── config.go            # Config loader
├── tidal/
│   └── client.go            # Tidal API magic
├── venues/
│   ├── types.go             # Data structures
│   ├── loader.go            # YAML venue loader
│   ├── scraper.go           # Scraper orchestration
│   └── scrapers.go          # Venue-specific scrapers
├── main.go                  # Main app
├── run.sh                   # 1Password helper script
├── venues.yaml              # Your venues
└── .env                     # Secrets (not committed)
```

## Technical Notes

### Rate Limiting

Tidal's API has strict rate limits. The app handles this gracefully:

- 500ms delay between artist searches
- 300ms delay between search and fetching tracks
- Auto-retry with 2-second backoff on rate limit errors
- Keeps going even if some artists get skipped

If you have 20 artists and end up with 15 tracks, that's normal. Some artists won't be on Tidal, some might get rate-limited, and that's okay.

### Artist Matching

When multiple artists share a name:
1. Prefers exact name match (case-insensitive)
2. Picks the one with highest popularity score
3. Falls back to first result if no exact match

This mostly gets it right, but sometimes "John Smith" the indie rocker gets confused with "John Smith" the jazz pianist. The algorithm tries its best.

## Troubleshooting

**"Failed to authenticate with Tidal"**
- Re-run `go run cmd/auth/main.go` to get fresh tokens
- Make sure you granted both `collection.read` and `playlists.write` scopes

**"No shows found"**
- Check your `venues.yaml` URLs are correct
- Try visiting the URLs in a browser to confirm they're working
- The venue might have changed their website layout (PRs welcome to fix scrapers!)

**Many artists showing "not found"**
- Some artists just aren't on Tidal yet
- Rate limiting might skip a few (this is expected)
- Multi-artist shows like "Band A w/ Band B" might not parse perfectly

**Playlist has fewer tracks than artists**
- Totally normal! Rate limits, missing artists, regional restrictions all play a role
- The playlist will still give you plenty to discover

## Contributing

Want to add a new venue scraper? Love PRs:

1. Implement the `Scraper` interface in `venues/scrapers.go`
2. Add your scraper type to the `GetScraper()` function
3. Test it with a real venue URL
4. Submit a PR with a description of the venue(s) it supports

Feature ideas, bug reports, venue requests—all welcome in Issues.

## Philosophy

Music discovery should be an adventure, not an algorithm. The best shows aren't always the most popular ones. Your next favorite band might be playing next Tuesday at that venue you walk past every day.

Let TideTable do the legwork. You do the discovering.

## License

MIT

## Credits

Built with love (and Claude Code) for people who still believe live music is magic.

---

**Now go discover something amazing.** 🎸
