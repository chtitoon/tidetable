# TideTable

Automatically sync concerts from your favorite Tidal artists to Google Calendar using GitHub Actions.

## Features

- Fetches your favorite artists from Tidal
- Finds upcoming concerts via Bandsintown API
- Adds concerts to your Google Calendar (with duplicate checking)
- Runs automatically via GitHub Actions (daily at 9 AM UTC)
- Minimalist CLI application with no UI

## Setup

### 1. Prerequisites

- Go 1.23+ installed locally (for initial setup)
- A Tidal account with favorite artists
- A Google account
- A GitHub account

### 2. Clone and Install

```bash
git clone https://github.com/chtitoon/tidetable.git
cd tidetable
go mod download
```

### 3. Bandsintown API

The Bandsintown API requires an `app_id` parameter, but for public read-only access (like fetching concert info), it doesn't strictly validate this. You can use any identifier like your app name or email address.

Set `BANDSINTOWN_APP_ID` to something like `tidetable` or your email.

### 4. Get Tidal Developer Access

1. Visit https://developer.tidal.com/
2. Create an account and register a new application
3. Set the **Redirect URI** to: `http://localhost:8080/callback`
4. Note your Client ID and Client Secret
5. Run the authentication helper:

```bash
TIDAL_CLIENT_ID=your_client_id TIDAL_CLIENT_SECRET=your_client_secret go run cmd/auth/main.go
```

6. Your browser will open automatically for Tidal login
7. Log in and authorize the application
8. The tokens will be displayed in your terminal - save them for later

### 5. Setup Google Calendar Access

**Simple OAuth - No Service Account Required!** Just like Tidal, this is a one-time browser login.

#### Step 1: Create OAuth Credentials (One-time, ~5 minutes)

Google requires you to register your app through Google Cloud Console to get OAuth credentials. Don't worry - it's free and straightforward:

1. **Go to [Google Cloud Console](https://console.cloud.google.com/)**
2. **Create a project:**
   - Click the project dropdown at the top
   - Click "New Project"
   - Name it anything (e.g., "TideTable")
   - Click "Create"
3. **Enable Calendar API:**
   - In the search bar, type "Google Calendar API"
   - Click on it and click "Enable"
4. **Configure OAuth consent screen:**
   - Go to "APIs & Services" → "OAuth consent screen" (left sidebar)
   - Choose "External" user type
   - Click "Create"
   - Fill in:
     - App name: "TideTable" (or anything)
     - User support email: your email
     - Developer contact: your email
   - Click "Save and Continue"
   - Click "Add or Remove Scopes"
     - Search for "calendar" and check the box for `.../auth/calendar` scope
     - Click "Update" then "Save and Continue"
   - **Add Test Users:**
     - Click "Add Users"
     - Enter YOUR Google email address
     - Click "Add" then "Save and Continue"
   - Click "Back to Dashboard"
5. **Create OAuth credentials:**
   - Go to "Credentials" (left sidebar)
   - Click "Create Credentials" → "OAuth client ID"
   - Application type: **Desktop app**
   - Name: "TideTable Desktop"
   - Click "Create"
6. **Download credentials:**
   - Click the download button (⬇) next to your newly created OAuth client
   - Save the downloaded file as `credentials.json` in your project root directory

**Note:** The `credentials.json` file is just your OAuth client config (not a secret). It tells Google this is your app.

#### Step 2: Authenticate Your Google Account (One-time, ~1 minute)

Run the setup script:

```bash
go run cmd/setup-calendar/main.go
```

- Browser opens automatically
- Log in to Google
- Approve calendar access (Google will show a warning - click "Advanced" → "Go to TideTable (unsafe)" - this is normal for personal apps)
- Done! A `token.json` file is created

**The `token.json` contains your access token and refresh token - it auto-renews and lasts forever!**

**Important:**
- `credentials.json` = OAuth client config (can commit to git)
- `token.json` = Your personal access token (do NOT commit, already in .gitignore)

### 6. Setup Google Maps API (for location filtering)

The app automatically fetches your home location from Google Maps to filter concerts within your chosen radius.

1. **In the same Google Cloud Console project:**
   - Search for "Geocoding API"
   - Click "Enable"
2. **Create an API key:**
   - Go to "Credentials" (left sidebar)
   - Click "Create Credentials" → "API key"
   - Copy the API key
   - (Optional) Click "Restrict Key" and limit it to "Geocoding API" for security
3. **Save "Home" in Google Maps:**
   - Go to [Google Maps](https://maps.google.com)
   - Search for your home address
   - Click "Save" → "Labeled" → Select or create "Home"

The app will automatically fetch your home coordinates and filter concerts within your specified radius!

### 7. Configure Environment Variables

Copy `.env.example` to `.env`:

```bash
cp .env.example .env
```

Edit `.env` with your credentials:

```env
BANDSINTOWN_APP_ID=tidetable
TIDAL_CLIENT_ID=your_tidal_client_id
TIDAL_CLIENT_SECRET=your_tidal_client_secret
TIDAL_ACCESS_TOKEN=your_tidal_access_token
TIDAL_REFRESH_TOKEN=your_tidal_refresh_token
GOOGLE_CALENDAR_TOKEN=<contents of token.json as single line>
GOOGLE_CALENDAR_ID=primary
GOOGLE_MAPS_API_KEY=your_google_maps_api_key
RADIUS_KM=100
```

**Configuration details:**

- `BANDSINTOWN_APP_ID`: Can be any identifier (e.g., `tidetable` or your email)
- `TIDAL_CLIENT_ID` / `TIDAL_CLIENT_SECRET`: From Tidal Developer Portal
- `TIDAL_ACCESS_TOKEN` / `TIDAL_REFRESH_TOKEN`: Generated by `go run cmd/auth/main.go`
- `GOOGLE_CALENDAR_TOKEN`: Copy the entire contents of `token.json` as a single line (all the JSON on one line)
- `GOOGLE_CALENDAR_ID`:
  - Use `primary` for your main calendar (recommended)
  - Or get a specific calendar ID:
    1. Go to [Google Calendar](https://calendar.google.com)
    2. Click the three dots next to a calendar → "Settings and sharing"
    3. Scroll to "Integrate calendar" and copy the Calendar ID
    4. It looks like: `abc123@group.calendar.google.com`
- `GOOGLE_MAPS_API_KEY`: API key from Google Cloud Console (Geocoding API enabled)
- `RADIUS_KM`: Distance in kilometers to search for concerts around your home (default: 100)

### 7. Test Locally

```bash
go run main.go
```

You should see output showing artists being checked and events being added to your calendar.

### 8. Deploy to GitHub Actions

1. Push your code to GitHub (make sure `credentials.json` and `token.json` are NOT committed!)
2. Add the `credentials.json` content to your repository:
   - Commit it directly (it's not secret, just OAuth client config)
3. Add the following secrets to your GitHub repository (Settings → Secrets and variables → Actions):
   - `BANDSINTOWN_APP_ID`
   - `TIDAL_CLIENT_ID`
   - `TIDAL_CLIENT_SECRET`
   - `TIDAL_ACCESS_TOKEN`
   - `TIDAL_REFRESH_TOKEN`
   - `GOOGLE_CALENDAR_TOKEN` (entire contents of token.json)
   - `GOOGLE_CALENDAR_ID` (usually "primary")

The workflow will run automatically every day at 9 AM UTC, or you can trigger it manually from the Actions tab.

## Project Structure

```
tidetable/
├── .github/
│   └── workflows/
│       └── sync-concerts.yml   # GitHub Actions workflow
├── bandsintown/
│   └── client.go               # Bandsintown API client
├── calendar/
│   └── client.go               # Google Calendar client
├── cmd/
│   ├── auth/
│   │   └── main.go             # Tidal authentication helper
│   └── setup-calendar/
│       └── main.go             # Google Calendar setup helper
├── config/
│   └── config.go               # Configuration loader
├── tidal/
│   └── client.go               # Tidal API client with OAuth device flow
├── main.go                     # Main application
├── .env.example                # Example environment variables
├── .gitignore                  # Git ignore file
└── README.md                   # This file
```

## How It Works

1. **Tidal Integration**: Uses OAuth 2.1 device flow to authenticate and fetch your favorite artists
2. **Concert Discovery**: For each artist, queries Bandsintown API for upcoming concerts
3. **Duplicate Prevention**: Checks your calendar for existing events before adding new ones
4. **Auto-sync**: GitHub Actions runs the sync daily, keeping your calendar updated

## Customization

### Change Sync Schedule

Edit `.github/workflows/sync-concerts.yml` and modify the cron expression:

```yaml
schedule:
  - cron: '0 9 * * *'  # Daily at 9 AM UTC
```

### Change Calendar

Set `GOOGLE_CALENDAR_ID` to a different calendar ID (found in Google Calendar settings).

## Troubleshooting

### "Failed to fetch favorite artists"

- Check your Tidal access token is valid
- Re-run `go run cmd/auth/main.go` to get fresh tokens

### "Unable to read credentials.json"

- Make sure `credentials.json` is in the project root
- Verify it's properly formatted JSON

### "Request failed with status 401"

- Your Google Calendar token may have expired
- Re-run `go run cmd/setup-calendar/main.go` to refresh

### No concerts found

- Verify your Tidal favorite artists list is not empty
- Check Bandsintown has data for those artists

## License

MIT

## Contributing

Pull requests welcome!
