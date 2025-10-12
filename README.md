# TideTable

Automatically creates a weekly Tidal playlist with artists performing at your favorite venues.

## Features

- Scrapes venue websites for upcoming shows
- Searches Tidal for each performing artist
- Creates/updates a weekly playlist with one track per artist
- Handles rate limiting gracefully with retries and delays
- Supports multiple artist profiles (picks most popular by default)
- Runs automatically via GitHub Actions every Friday
- Minimalist CLI application

## How It Works

1. **Venue Scraping**: Scrapes configured venue websites for upcoming shows in the next 7 days
2. **Artist Discovery**: Extracts artist names from show listings
3. **Tidal Search**: Searches Tidal for each artist, preferring exact name matches with highest popularity
4. **Playlist Creation**: Creates or updates a playlist with one track per artist
5. **Weekly Updates**: GitHub Actions runs every Friday to refresh the playlist

## Setup

### 1. Prerequisites

- Go 1.21+ installed locally (for initial setup)
- A Tidal account
- A GitHub account (for automated runs)
- 1Password CLI (optional, for secret management)

### 2. Clone and Install

```bash
git clone https://github.com/chtitoon/tidetable.git
cd tidetable
go mod download
```

### 3. Get Tidal Developer Access

1. Visit https://developer.tidal.com/
2. Create an account and register a new application
3. Set the **Redirect URI** to: `http://localhost:8080/callback`
4. Note your Client ID and Client Secret
5. Run the authentication helper:

```bash
TIDAL_CLIENT_ID=your_client_id TIDAL_CLIENT_SECRET=your_client_secret go run cmd/auth/main.go
```

6. Your browser will open automatically for Tidal login
7. Log in and authorize the application with `collection.read playlists.write` scopes
8. The tokens will be displayed in your terminal - save them for later

### 4. Configure Venues

Edit `venues.yaml` to add your favorite venues:

```yaml
venues:
  - name: "Mercury Lounge / Bowery Ballroom"
    url: "https://mercuryeastpresents.com/calendar-mr/"
    location: "New York, NY"
    scraper_type: "mercury_east"

  # Add more venues here
  # - name: "Your Venue"
  #   url: "https://example.com/calendar"
  #   location: "City, State"
  #   scraper_type: "generic"
```

**Supported Scraper Types:**
- `mercury_east`: Mercury East Presents venues (Mercury Lounge, Bowery Ballroom, etc.)
- `generic`: Basic scraper (needs customization for specific venues)

### 5. Configure Environment Variables

Create a `.env` file:

```env
TIDAL_CLIENT_ID=your_tidal_client_id
TIDAL_CLIENT_SECRET=your_tidal_client_secret
TIDAL_ACCESS_TOKEN=your_tidal_access_token
TIDAL_REFRESH_TOKEN=your_tidal_refresh_token
PLAYLIST_NAME="This Week's Shows"
VENUES_FILE="venues.yaml"
```

**Using 1Password (Recommended):**

If you use 1Password, you can store secrets there and use the CLI to inject them:

```bash
op run --env-file=.env -- go run main.go
```

Store your secrets in 1Password and reference them in `.env`:
```env
TIDAL_CLIENT_ID="op://Private/Tidal/tidetable/clientId"
TIDAL_CLIENT_SECRET="op://Private/Tidal/tidetable/clientSecret"
TIDAL_ACCESS_TOKEN="op://Private/Tidal/tidetable/accessToken"
TIDAL_REFRESH_TOKEN="op://Private/Tidal/tidetable/refreshToken"
```

### 6. Test Locally

```bash
./run.sh
```

Or without 1Password:

```bash
go run main.go
```

You should see output showing venues being scraped, artists being searched, and the playlist being created/updated.

### 7. Deploy to GitHub Actions

1. Add the following secrets to your GitHub repository (Settings → Secrets and variables → Actions):
   - `TIDAL_CLIENT_ID`
   - `TIDAL_CLIENT_SECRET`
   - `TIDAL_ACCESS_TOKEN`
   - `TIDAL_REFRESH_TOKEN`

2. The workflow (`.github/workflows/weekly-playlist.yml`) will run automatically every Friday at 9 AM UTC

3. You can also trigger it manually from the Actions tab

## Project Structure

```
tidetable/
├── .github/
│   └── workflows/
│       └── weekly-playlist.yml  # GitHub Actions workflow for Friday runs
├── cmd/
│   └── auth/
│       └── main.go              # Tidal authentication helper
├── config/
│   └── config.go                # Configuration loader
├── tidal/
│   └── client.go                # Tidal API client with OAuth 2.0 PKCE
├── venues/
│   ├── types.go                 # Venue and Show data structures
│   ├── loader.go                # YAML venue configuration loader
│   ├── scraper.go               # Scraper interface and orchestration
│   └── scrapers.go              # Venue-specific scraper implementations
├── main.go                      # Main application
├── run.sh                       # Helper script for 1Password integration
├── venues.yaml                  # Venue configuration
├── .env                         # Environment variables (not committed)
├── .gitignore                   # Git ignore file
└── README.md                    # This file
```

## Rate Limiting

The Tidal API has strict rate limits. The app handles this by:

- Adding 500ms delay between artist searches
- Adding 300ms delay between search and track fetching
- Retrying with 2-second backoff on 429 (Too Many Requests) responses
- Gracefully continuing when rate limited (some artists may be skipped)

For 19 unique artists, expect about 14-15 tracks in the final playlist due to rate limits and artists not found on Tidal.

## Artist Matching

When multiple artists have the same name on Tidal, the app:

1. Prefers exact name matches (case-insensitive)
2. Selects the artist with highest popularity score
3. Falls back to the first result if no exact match

This ensures the correct artist is selected when there are multiple profiles with the same name.

## Customization

### Change Sync Schedule

Edit `.github/workflows/weekly-playlist.yml` and modify the cron expression:

```yaml
schedule:
  - cron: '0 9 * * 5'  # Every Friday at 9 AM UTC
```

### Change Playlist Name

Set `PLAYLIST_NAME` in your `.env` file.

### Add More Venues

Edit `venues.yaml` and add additional venues with appropriate scraper types.

## Troubleshooting

### "Failed to authenticate with Tidal"

- Check your Tidal tokens are valid
- Re-run `go run cmd/auth/main.go` to get fresh tokens
- Ensure you granted both `collection.read` and `playlists.write` scopes

### "No shows found"

- Verify your `venues.yaml` URLs are correct
- Check the venue websites are accessible
- Ensure the scraper type matches the venue format

### Many artists showing "not found" or "✓ added 0 track(s)"

This is usually due to:
- **Rate limiting**: Tidal API has strict limits; the app handles this gracefully
- **Artist not on Tidal**: Some artists may not have a Tidal profile
- **Multiple artist profiles**: The app selects by popularity, but the highest-ranked profile might have no tracks

### Playlist has fewer tracks than artists found

This is normal due to:
- Rate limiting from Tidal API
- Multi-artist shows (e.g., "Artist A, Artist B") not found as single entity
- Artists with promotional text (e.g., "Artist Name - Tour Name") not matching Tidal profiles
- Some artists legitimately having no tracks available in your region

## Contributing

Pull requests welcome! To add a new venue scraper:

1. Implement the `Scraper` interface in `venues/scrapers.go`
2. Add the scraper type to `GetScraper()` switch statement
3. Test with your venue's URL

## License

MIT

## Credits

Built with Claude Code
