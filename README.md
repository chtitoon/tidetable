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

## License

MIT

## Credits

Built with (some) love (and Claude Code).
