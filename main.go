package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/chtitoon/tidetable/config"
	"github.com/chtitoon/tidetable/lescraper"
	"github.com/chtitoon/tidetable/tidal"
)

func main() {
	ctx := context.Background()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	fmt.Printf("Loading venues from: %s\n", cfg.VenuesFile)

	// Load venue names
	venueNames, err := config.LoadVenues(cfg.VenuesFile)
	if err != nil {
		log.Fatalf("Failed to load venues: %v", err)
	}

	fmt.Printf("Loaded %d venue(s): %s\n\n", len(venueNames), strings.Join(venueNames, ", "))

	// Calculate date range based on current day:
	// - If Friday: look at next Monday through next Sunday (upcoming week)
	// - Any other day: today through next Sunday (rest of current week)
	now := time.Now()
	weekday := now.Weekday()

	var startDate, endDate time.Time

	if weekday == time.Friday {
		// On Friday: look at next week (next Monday to next Sunday)
		daysUntilMonday := int(time.Monday - weekday)
		if daysUntilMonday <= 0 {
			daysUntilMonday += 7
		}
		startDate = now.AddDate(0, 0, daysUntilMonday)
		endDate = startDate.AddDate(0, 0, 6) // Sunday
		fmt.Printf("Scraping venues for shows from %s to %s (next week)...\n",
			startDate.Format("Mon Jan 2"), endDate.Format("Mon Jan 2, 2006"))
	} else {
		// Any other day: today through next Sunday
		startDate = now
		daysUntilSunday := int(time.Sunday - weekday)
		if daysUntilSunday < 0 {
			daysUntilSunday += 7
		}
		endDate = now.AddDate(0, 0, daysUntilSunday)
		fmt.Printf("Scraping venues for shows from %s to %s (rest of this week)...\n",
			startDate.Format("Mon Jan 2"), endDate.Format("Mon Jan 2, 2006"))
	}

	// Scrape all venues for shows
	shows, err := lescraper.ScrapeVenues(venueNames, startDate, endDate)
	if err != nil {
		log.Fatalf("Failed to scrape venues: %v", err)
	}

	fmt.Printf("\n=== Found %d show(s) this week ===\n\n", len(shows))

	if len(shows) == 0 {
		fmt.Println("No shows found for this week. Try again later!")
		return
	}

	// Normalize shows to extract individual artists
	fmt.Println("Normalizing show data and extracting artists...")
	normalizedShows := lescraper.NormalizeShows(shows)

	// Sort normalized shows by date (Monday first, then Tuesday, etc.)
	sort.Slice(normalizedShows, func(i, j int) bool {
		return normalizedShows[i].Date.Before(normalizedShows[j].Date)
	})

	// Print shows in chronological order
	totalArtists := 0
	for _, show := range normalizedShows {
		// Display all artists for this show
		artistList := strings.Join(show.Artists, ", ")
		fmt.Printf("📅 %s - %s at %s\n", show.Date.Format("Mon Jan 2"), artistList, show.Venue)
		if show.Price != "" {
			fmt.Printf("   💵 %s\n", show.Price)
		}
		fmt.Printf("   🔗 %s\n\n", show.URL)
		totalArtists += len(show.Artists)
	}

	// Initialize Tidal service
	fmt.Println("Connecting to Tidal...")
	auth := tidal.NewAuthenticator(cfg.TidalClientID, cfg.TidalClientSecret, "")
	auth.SetTokens(cfg.TidalAccessToken, cfg.TidalRefreshToken)
	tidalService := tidal.NewService(auth)

	// Deduplicate artists while preserving chronological order
	// Extract all artists from normalized shows
	uniqueArtists := make(map[string]bool)
	var artistsToSearch []string
	for _, show := range normalizedShows {
		for _, artist := range show.Artists {
			if !uniqueArtists[artist] {
				uniqueArtists[artist] = true
				artistsToSearch = append(artistsToSearch, artist)
			}
		}
	}

	fmt.Printf("\nExtracted %d unique artists from %d shows (%d total artist performances)\n",
		len(artistsToSearch), len(normalizedShows), totalArtists)

	// Search for each artist on Tidal and collect track IDs
	fmt.Println("Searching for artists on Tidal...")
	var trackIDs []string

	for i, artist := range artistsToSearch {
		// Add delay between requests to avoid rate limiting (except for first request)
		// Tidal API has strict rate limits, so we need to be conservative
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}

		fmt.Printf("  Searching for %s...", artist)

		artistID, err := tidalService.SearchArtist(ctx, artist)
		if err != nil {
			fmt.Printf(" not found\n")
			continue
		}

		// Delay between search and getting tracks to avoid rate limiting
		time.Sleep(300 * time.Millisecond)

		// Get top track for this artist (just 1 song)
		tracks, err := tidalService.GetArtistTopTracks(ctx, artistID, 1) // Get just 1 track
		if err != nil {
			fmt.Printf(" found but no tracks\n")
			continue
		}

		trackIDs = append(trackIDs, tracks...)
		fmt.Printf(" ✓ added %d track(s)\n", len(tracks))
	}

	if len(trackIDs) == 0 {
		fmt.Println("\nNo tracks found on Tidal. Cannot create playlist.")
		return
	}

	fmt.Printf("\n=== Creating playlist with %d tracks ===\n", len(trackIDs))

	// Get playlist name from config
	playlistName := cfg.PlaylistName

	// Get unique venues from the shows
	venueMap := make(map[string]bool)
	for _, show := range shows {
		venueMap[show.Venue] = true
	}
	var uniqueVenues []string
	for venueName := range venueMap {
		uniqueVenues = append(uniqueVenues, venueName)
	}

	// Format dates for description
	startDateStr := startDate.Format("Jan 2")
	endDateStr := endDate.Format("Jan 2")

	description := fmt.Sprintf("Shows for %s - %s at %s",
		startDateStr, endDateStr, strings.Join(uniqueVenues, ", "))

	// Create or update playlist
	playlistID, err := tidalService.CreateOrUpdatePlaylist(ctx, playlistName, description, trackIDs)
	if err != nil {
		log.Fatalf("Failed to create playlist: %v", err)
	}

	fmt.Printf("\n✅ Playlist '%s' created/updated successfully!\n", playlistName)
	fmt.Printf("🎵 Playlist ID: %s\n", playlistID)
	fmt.Printf("🔗 https://listen.tidal.com/playlist/%s\n", playlistID)
}
