package tidal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Service provides high-level operations for Tidal API
// Handles business logic, rate limiting, retries, and orchestration
type Service struct {
	client *Client
	auth   *Authenticator
}

// NewService creates a new Tidal service
func NewService(auth *Authenticator) *Service {
	return &Service{
		client: NewClient(auth),
		auth:   auth,
	}
}

// ===== Artist Search with Business Logic =====

// SearchArtist searches for an artist by name with intelligent matching
// - Handles rate limiting with retries
// - Prefers exact name matches (case-insensitive)
// - Selects artist with highest popularity when multiple matches exist
// - Returns artist ID on success
func (s *Service) SearchArtist(ctx context.Context, artistName string) (string, error) {
	artists, err := s.searchArtistWithRetry(ctx, artistName)
	if err != nil {
		return "", err
	}

	if len(artists) == 0 {
		return "", fmt.Errorf("no artist found for: %s", artistName)
	}

	// Find best match: exact name match with highest popularity
	searchLower := strings.ToLower(artistName)
	var bestMatch *Artist
	var firstArtist *Artist

	for i := range artists {
		artist := &artists[i]

		if firstArtist == nil {
			firstArtist = artist
		}

		// Check for exact match (case-insensitive)
		if strings.ToLower(artist.Name) == searchLower {
			if bestMatch == nil || artist.Popularity > bestMatch.Popularity {
				bestMatch = artist
			}
		}
	}

	// Return best exact match if found, otherwise first result
	if bestMatch != nil {
		return bestMatch.ID, nil
	}

	return firstArtist.ID, nil
}

// searchArtistWithRetry searches with automatic retry on rate limit
func (s *Service) searchArtistWithRetry(ctx context.Context, artistName string) ([]Artist, error) {
	artists, err := s.client.SearchArtists(ctx, artistName)

	// Handle 401 Unauthorized - try refreshing token
	if err != nil && strings.Contains(err.Error(), "401") {
		if refreshErr := s.auth.RefreshAccessToken(); refreshErr != nil {
			return nil, fmt.Errorf("token expired and refresh failed: %w", refreshErr)
		}
		// Retry with new token
		artists, err = s.client.SearchArtists(ctx, artistName)
	}

	// Handle 429 Too Many Requests - retry with backoff
	if err != nil && strings.Contains(err.Error(), "429") {
		time.Sleep(2 * time.Second)
		artists, err = s.client.SearchArtists(ctx, artistName)
	}

	return artists, err
}

// ===== Track Retrieval with Limits =====

// GetArtistTopTracks retrieves top tracks for an artist with limit
// - Handles rate limiting and retries
// - Limits number of tracks returned
// - Returns track IDs
func (s *Service) GetArtistTopTracks(ctx context.Context, artistID string, limit int) ([]string, error) {
	trackIDs, err := s.getArtistTracksWithRetry(ctx, artistID)
	if err != nil {
		return nil, err
	}

	// Limit number of tracks
	if len(trackIDs) > limit {
		trackIDs = trackIDs[:limit]
	}

	return trackIDs, nil
}

// getArtistTracksWithRetry gets tracks with automatic retry on rate limit
func (s *Service) getArtistTracksWithRetry(ctx context.Context, artistID string) ([]string, error) {
	trackIDs, err := s.client.GetArtistTracks(ctx, artistID)

	// Handle 401 Unauthorized - try refreshing token
	if err != nil && strings.Contains(err.Error(), "401") {
		if refreshErr := s.auth.RefreshAccessToken(); refreshErr != nil {
			return nil, fmt.Errorf("token expired and refresh failed: %w", refreshErr)
		}
		// Retry with new token
		trackIDs, err = s.client.GetArtistTracks(ctx, artistID)
	}

	// Handle 429 Too Many Requests - retry with backoff
	if err != nil && strings.Contains(err.Error(), "429") {
		time.Sleep(2 * time.Second)
		trackIDs, err = s.client.GetArtistTracks(ctx, artistID)
	}

	return trackIDs, err
}

// ===== Playlist Orchestration =====

// CreateOrUpdatePlaylist creates a new playlist or updates existing one by name
// - Searches for existing playlist with the same name
// - If found: clears it and adds new tracks
// - If not found: creates new playlist and adds tracks
// - Handles batch operations (max 20 tracks per API call)
func (s *Service) CreateOrUpdatePlaylist(ctx context.Context, name, description string, trackIDs []string) (string, error) {
	// Try to find existing playlist
	existingID, err := s.findPlaylistByName(ctx, name)

	if err == nil && existingID != "" {
		// Playlist exists - update it
		fmt.Printf("Found existing playlist, updating...\n")

		if err := s.clearPlaylist(ctx, existingID); err != nil {
			return "", fmt.Errorf("clearing playlist: %w", err)
		}

		if err := s.addTracksInBatches(ctx, existingID, trackIDs); err != nil {
			return existingID, fmt.Errorf("adding tracks: %w", err)
		}

		return existingID, nil
	}

	// Playlist doesn't exist - create new one
	fmt.Printf("Creating new playlist...\n")

	playlist, err := s.createPlaylistWithRetry(ctx, name, description)
	if err != nil {
		return "", err
	}

	if err := s.addTracksInBatches(ctx, playlist.ID, trackIDs); err != nil {
		return playlist.ID, fmt.Errorf("adding tracks: %w", err)
	}

	return playlist.ID, nil
}

// findPlaylistByName searches user's playlists for one with matching name
func (s *Service) findPlaylistByName(ctx context.Context, name string) (string, error) {
	// Get current user ID
	user, err := s.getUserWithRetry(ctx)
	if err != nil {
		return "", fmt.Errorf("getting user: %w", err)
	}

	// Get user's playlists
	playlists, err := s.getUserPlaylistsWithRetry(ctx, user.ID)
	if err != nil {
		return "", fmt.Errorf("getting playlists: %w", err)
	}

	// Find by name
	for _, playlist := range playlists {
		if playlist.Name == name {
			return playlist.ID, nil
		}
	}

	return "", fmt.Errorf("playlist not found")
}

// clearPlaylist removes all tracks from a playlist
func (s *Service) clearPlaylist(ctx context.Context, playlistID string) error {
	items, err := s.client.GetPlaylistItems(ctx, playlistID)
	if err != nil {
		return fmt.Errorf("getting playlist items: %w", err)
	}

	if len(items) == 0 {
		return nil // Nothing to clear
	}

	if err := s.client.DeletePlaylistItems(ctx, playlistID, items); err != nil {
		return fmt.Errorf("deleting items: %w", err)
	}

	return nil
}

// addTracksInBatches adds tracks to playlist in batches of 20 (API limit)
func (s *Service) addTracksInBatches(ctx context.Context, playlistID string, trackIDs []string) error {
	const batchSize = 20

	for i := 0; i < len(trackIDs); i += batchSize {
		end := i + batchSize
		if end > len(trackIDs) {
			end = len(trackIDs)
		}
		batch := trackIDs[i:end]

		if err := s.client.AddTracksToPlaylist(ctx, playlistID, batch); err != nil {
			return fmt.Errorf("adding batch %d-%d: %w", i, end, err)
		}
	}

	return nil
}

// ===== Helper Methods with Retry Logic =====

// getUserWithRetry gets current user with retry on auth failure
func (s *Service) getUserWithRetry(ctx context.Context) (*User, error) {
	user, err := s.client.GetCurrentUser(ctx)

	// Handle 401 Unauthorized - try refreshing token
	if err != nil && strings.Contains(err.Error(), "401") {
		if refreshErr := s.auth.RefreshAccessToken(); refreshErr != nil {
			return nil, fmt.Errorf("token expired and refresh failed: %w", refreshErr)
		}
		// Retry with new token
		user, err = s.client.GetCurrentUser(ctx)
	}

	return user, err
}

// getUserPlaylistsWithRetry gets user playlists with retry on auth failure
func (s *Service) getUserPlaylistsWithRetry(ctx context.Context, userID string) ([]Playlist, error) {
	playlists, err := s.client.GetUserPlaylists(ctx, userID)

	// Handle 401 Unauthorized - try refreshing token
	if err != nil && strings.Contains(err.Error(), "401") {
		if refreshErr := s.auth.RefreshAccessToken(); refreshErr != nil {
			return nil, fmt.Errorf("token expired and refresh failed: %w", refreshErr)
		}
		// Retry with new token
		playlists, err = s.client.GetUserPlaylists(ctx, userID)
	}

	return playlists, err
}

// createPlaylistWithRetry creates playlist with retry on auth failure
func (s *Service) createPlaylistWithRetry(ctx context.Context, name, description string) (*Playlist, error) {
	playlist, err := s.client.CreatePlaylist(ctx, name, description)

	// Handle 401 Unauthorized - try refreshing token
	if err != nil && strings.Contains(err.Error(), "401") {
		if refreshErr := s.auth.RefreshAccessToken(); refreshErr != nil {
			return nil, fmt.Errorf("token expired and refresh failed: %w", refreshErr)
		}
		// Retry with new token
		playlist, err = s.client.CreatePlaylist(ctx, name, description)
	}

	return playlist, err
}

// ===== Compatibility Methods =====

// For backwards compatibility with existing code
// These delegate to the Authenticator

// SetTokens sets the access and refresh tokens
func (s *Service) SetTokens(accessToken, refreshToken string) {
	s.auth.SetTokens(accessToken, refreshToken)
}

// GetTokens returns the current access and refresh tokens
func (s *Service) GetTokens() (accessToken, refreshToken string) {
	return s.auth.GetTokens()
}

// RefreshAccessToken refreshes the access token
func (s *Service) RefreshAccessToken() error {
	return s.auth.RefreshAccessToken()
}

// ===== Legacy Client Methods =====
// For backwards compatibility - these will be removed once main.go is updated

// NewClient creates a legacy client (deprecated)
func NewLegacyClient(clientID, clientSecret, redirectURI string) *Service {
	auth := NewAuthenticator(clientID, clientSecret, redirectURI)
	return NewService(auth)
}

// SearchArtistLegacy provides backwards compatibility
func (s *Service) SearchArtistLegacy(ctx context.Context, artistName string) (string, error) {
	return s.SearchArtist(ctx, artistName)
}

// GetArtistTopTracksLegacy provides backwards compatibility
func (s *Service) GetArtistTopTracksLegacy(ctx context.Context, artistID string, limit int) ([]string, error) {
	return s.GetArtistTopTracks(ctx, artistID, limit)
}

// CreateOrUpdatePlaylistLegacy provides backwards compatibility
func (s *Service) CreateOrUpdatePlaylistLegacy(ctx context.Context, name, description string, trackIDs []string) (string, error) {
	return s.CreateOrUpdatePlaylist(ctx, name, description, trackIDs)
}

// StatusCode helper to check error status codes
func isStatusCode(err error, code int) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), fmt.Sprintf("%d", code))
}

// IsUnauthorized checks if error is 401
func IsUnauthorized(err error) bool {
	return isStatusCode(err, http.StatusUnauthorized)
}

// IsTooManyRequests checks if error is 429
func IsTooManyRequests(err error) bool {
	return isStatusCode(err, http.StatusTooManyRequests)
}

// ===== Artist Export: retry helper and collection reads =====

// Retry policy for the export calls. Package variables so tests can shrink them.
var (
	retryMaxAttempts = 6
	retryBaseDelay   = 2 * time.Second
	retryMaxDelay    = 60 * time.Second
	// politeDelay is the pause after every successful export call.
	politeDelay = 300 * time.Millisecond
	sleep       = func(ctx context.Context, d time.Duration) error {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}
)

// withRetry runs fn with the shared retry policy for export calls:
//   - 401: refresh the access token once, then retry
//   - 429 and 5xx: exponential backoff (honouring Retry-After), up to retryMaxAttempts
//   - anything else (including other 4xx): returned immediately
func withRetry[T any](ctx context.Context, s *Service, fn func() (T, error)) (T, error) {
	refreshed := false
	delay := retryBaseDelay
	for attempt := 1; ; attempt++ {
		result, err := fn()
		if err == nil {
			return result, sleep(ctx, politeDelay)
		}

		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			return result, err
		}

		switch {
		case apiErr.StatusCode == http.StatusUnauthorized && !refreshed:
			refreshed = true
			if rerr := s.auth.RefreshAccessToken(); rerr != nil {
				return result, fmt.Errorf("token expired and refresh failed: %w", rerr)
			}
			attempt-- // a refresh doesn't use up a backoff attempt
			continue

		case apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= 500:
			if attempt >= retryMaxAttempts {
				return result, fmt.Errorf("giving up after %d attempts: %w", attempt, err)
			}
			wait := delay
			if apiErr.RetryAfter > wait {
				wait = apiErr.RetryAfter
			}
			if wait > retryMaxDelay {
				wait = retryMaxDelay
			}
			if serr := sleep(ctx, wait); serr != nil {
				return result, serr
			}
			delay *= 2

		default:
			return result, err
		}
	}
}

// FetchCollection reads every page of one of the user's collections by
// following links.next to the end. kind is "artists", "albums" or "tracks".
// Albums and tracks include their artists so the main artist can be attributed.
func (s *Service) FetchCollection(ctx context.Context, kind string) (Collection, error) {
	include := "items.artists"
	if kind == "artists" {
		include = "items"
	}

	col := Collection{Included: map[string]Resource{}}
	seen := map[string]bool{}
	for path := CollectionItemsPath(kind, include); path != ""; {
		if seen[path] {
			return col, fmt.Errorf("pagination loop in %s collection at %s", kind, path)
		}
		seen[path] = true

		page, err := withRetry(ctx, s, func() (*Page, error) { return s.client.GetPage(ctx, path) })
		if err != nil {
			return col, fmt.Errorf("reading %s collection: %w", kind, err)
		}
		col.Items = append(col.Items, page.Data...)
		for _, r := range page.Included {
			col.Included[r.Type+":"+r.ID] = r
		}
		path = page.Links.Next
	}
	return col, nil
}

// SimilarArtists returns the first page of similar artists for an artist.
// A 404 (artist unavailable in the catalogue) yields an empty list, not an error.
func (s *Service) SimilarArtists(ctx context.Context, artistID string) ([]Artist, error) {
	artists, err := withRetry(ctx, s, func() ([]Artist, error) { return s.client.GetSimilarArtists(ctx, artistID) })
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	return artists, err
}
