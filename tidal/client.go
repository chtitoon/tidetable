package tidal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	authURL     = "https://login.tidal.com/authorize"
	tokenURL    = "https://auth.tidal.com/v1/oauth2/token"
	apiBaseURL  = "https://openapi.tidal.com"
)

type Client struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
	redirectURI  string
	accessToken  string
	refreshToken string
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

type Artist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Track struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type favoritesResponse struct {
	Data []Artist `json:"data"`
}

func NewClient(clientID, clientSecret, redirectURI string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
	}
}

// GenerateAuthURL generates the authorization URL with PKCE
func (c *Client) GenerateAuthURL() (fullAuthURL, codeVerifier string, err error) {
	// Generate code verifier and challenge for PKCE
	codeVerifier, err = generateCodeVerifier()
	if err != nil {
		return "", "", fmt.Errorf("generating code verifier: %w", err)
	}

	codeChallenge := generateCodeChallenge(codeVerifier)

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", c.clientID)
	params.Set("redirect_uri", c.redirectURI)
	params.Set("scope", "collection.read playlists.write")
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")

	fullAuthURL = fmt.Sprintf("%s?%s", authURL, params.Encode())
	return fullAuthURL, codeVerifier, nil
}

// ExchangeCodeForToken exchanges authorization code for access token
func (c *Client) ExchangeCodeForToken(authCode, codeVerifier string) error {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", authCode)
	data.Set("client_id", c.clientID)
	data.Set("client_secret", c.clientSecret)
	data.Set("redirect_uri", c.redirectURI)
	data.Set("code_verifier", codeVerifier)
	data.Set("scope", "collection.read playlists.write")

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("creating token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("decoding token response: %w", err)
	}

	c.accessToken = tokenResp.AccessToken
	c.refreshToken = tokenResp.RefreshToken

	return nil
}

// SetTokens sets existing access and refresh tokens
func (c *Client) SetTokens(accessToken, refreshToken string) {
	c.accessToken = accessToken
	c.refreshToken = refreshToken
}

// RefreshAccessToken refreshes the access token using the refresh token
func (c *Client) RefreshAccessToken() error {
	data := url.Values{}
	data.Set("client_id", c.clientID)
	data.Set("client_secret", c.clientSecret)
	data.Set("refresh_token", c.refreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("creating refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("decoding refresh response: %w", err)
	}

	c.accessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		c.refreshToken = tokenResp.RefreshToken
	}

	return nil
}

// GetTokens returns the current access and refresh tokens
func (c *Client) GetTokens() (accessToken, refreshToken string) {
	return c.accessToken, c.refreshToken
}

// GetFavoriteArtists retrieves the user's favorite artists from Tidal
func (c *Client) GetFavoriteArtists() ([]Artist, error) {
	if c.accessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}

	// First, get the user ID from /v2/users/me
	meReq, err := http.NewRequest("GET", apiBaseURL+"/v2/users/me", nil)
	if err != nil {
		return nil, fmt.Errorf("creating me request: %w", err)
	}

	meReq.Header.Set("Authorization", "Bearer "+c.accessToken)
	meReq.Header.Set("Accept", "application/vnd.api+json")

	meResp, err := c.httpClient.Do(meReq)
	if err != nil {
		return nil, fmt.Errorf("executing me request: %w", err)
	}
	defer meResp.Body.Close()

	if meResp.StatusCode == http.StatusUnauthorized {
		// Try to refresh token
		if err := c.RefreshAccessToken(); err != nil {
			return nil, fmt.Errorf("token expired and refresh failed: %w", err)
		}
		// Retry with new token
		return c.GetFavoriteArtists()
	}

	if meResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(meResp.Body)
		return nil, fmt.Errorf("me request failed with status %d: %s", meResp.StatusCode, string(body))
	}

	var meData struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(meResp.Body).Decode(&meData); err != nil {
		return nil, fmt.Errorf("decoding me response: %w", err)
	}

	userID := meData.Data.ID

	// Now try to get favorites using /v2/userCollections/{id}/relationships/artists
	favReq, err := http.NewRequest("GET", apiBaseURL+"/v2/userCollections/"+userID+"/relationships/artists", nil)
	if err != nil {
		return nil, fmt.Errorf("creating favorites request: %w", err)
	}

	favReq.Header.Set("Authorization", "Bearer "+c.accessToken)
	favReq.Header.Set("Accept", "application/vnd.api+json")

	favResp, err := c.httpClient.Do(favReq)
	if err != nil {
		return nil, fmt.Errorf("executing favorites request: %w", err)
	}
	defer favResp.Body.Close()

	if favResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(favResp.Body)
		return nil, fmt.Errorf("favorites request failed with status %d: %s", favResp.StatusCode, string(body))
	}

	// Parse the relationships response to get artist IDs
	var favData struct {
		Data []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"data"`
		Links struct {
			Next string `json:"next"`
		} `json:"links"`
	}
	if err := json.NewDecoder(favResp.Body).Decode(&favData); err != nil {
		return nil, fmt.Errorf("decoding favorites response: %w", err)
	}

	// Now fetch each artist's details
	var artists []Artist
	fmt.Printf("Fetching details for %d artists...\n", len(favData.Data))
	for _, item := range favData.Data {
		// Add countryCode query parameter (US as default)
		artistReq, err := http.NewRequest("GET", apiBaseURL+"/v2/artists/"+item.ID+"?countryCode=US", nil)
		if err != nil {
			return nil, fmt.Errorf("creating artist request: %w", err)
		}

		artistReq.Header.Set("Authorization", "Bearer "+c.accessToken)
		artistReq.Header.Set("Accept", "application/vnd.api+json")

		artistResp, err := c.httpClient.Do(artistReq)
		if err != nil {
			return nil, fmt.Errorf("executing artist request: %w", err)
		}

		if artistResp.StatusCode == http.StatusOK {
			var artistData struct {
				Data struct {
					ID         string `json:"id"`
					Type       string `json:"type"`
					Attributes struct {
						Name string `json:"name"`
					} `json:"attributes"`
				} `json:"data"`
			}
			if err := json.NewDecoder(artistResp.Body).Decode(&artistData); err != nil {
				artistResp.Body.Close()
				return nil, fmt.Errorf("decoding artist response: %w", err)
			}
			artists = append(artists, Artist{
				ID:   artistData.Data.ID,
				Name: artistData.Data.Attributes.Name,
			})
		} else {
			body, _ := io.ReadAll(artistResp.Body)
			fmt.Printf("Failed to fetch artist %s: status %d: %s\n", item.ID, artistResp.StatusCode, string(body))
		}
		artistResp.Body.Close()

		// Small delay to avoid rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	// TODO: Handle pagination if needed (check favData.Links.Next)

	return artists, nil
}

// Helper functions for PKCE

func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func generateCodeChallenge(verifier string) string {
	h := sha256.New()
	h.Write([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// SearchArtist searches for an artist by name and returns their ID
func (c *Client) SearchArtist(ctx context.Context, artistName string) (string, error) {
	if c.accessToken == "" {
		return "", fmt.Errorf("not authenticated")
	}

	// URL encode the artist name for the path
	encodedName := url.PathEscape(artistName)

	// Search endpoint: GET /searchResults/{query}?countryCode=US&include=artists
	reqURL := fmt.Sprintf("%s/v2/searchResults/%s?countryCode=US&include=artists", apiBaseURL, encodedName)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/vnd.api+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		// Try to refresh token
		if err := c.RefreshAccessToken(); err != nil {
			return "", fmt.Errorf("token expired and refresh failed: %w", err)
		}
		// Retry with new token
		return c.SearchArtist(ctx, artistName)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("search failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to extract artist ID from included data
	var searchResp struct {
		Included []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Attributes struct {
				Name       string  `json:"name"`
				Popularity float64 `json:"popularity"`
			} `json:"attributes"`
		} `json:"included"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	// Find artist in included data
	// For exact matches, prefer the one with highest popularity
	searchLower := strings.ToLower(artistName)
	var bestMatch string
	var bestPopularity float64
	var firstArtistID string

	for _, item := range searchResp.Included {
		if item.Type == "artists" {
			if firstArtistID == "" {
				firstArtistID = item.ID
			}
			// Check for exact match (case insensitive)
			if strings.ToLower(item.Attributes.Name) == searchLower {
				// Prefer the artist with highest popularity
				if bestMatch == "" || item.Attributes.Popularity > bestPopularity {
					bestMatch = item.ID
					bestPopularity = item.Attributes.Popularity
				}
			}
		}
	}

	// Return best exact match if found
	if bestMatch != "" {
		return bestMatch, nil
	}

	// Fall back to first artist in results
	if firstArtistID != "" {
		return firstArtistID, nil
	}

	return "", fmt.Errorf("no artist found for: %s", artistName)
}

// GetArtistTopTracks gets the top tracks for an artist
func (c *Client) GetArtistTopTracks(ctx context.Context, artistID string, limit int) ([]string, error) {
	if c.accessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}

	// Endpoint: GET /artists/{id}/relationships/tracks?countryCode=US&collapseBy=FINGERPRINT&include=tracks
	reqURL := fmt.Sprintf("%s/v2/artists/%s/relationships/tracks?countryCode=US&collapseBy=FINGERPRINT&include=tracks", apiBaseURL, artistID)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/vnd.api+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		// Try to refresh token
		if err := c.RefreshAccessToken(); err != nil {
			return nil, fmt.Errorf("token expired and refresh failed: %w", err)
		}
		// Retry with new token
		return c.GetArtistTopTracks(ctx, artistID, limit)
	}

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		// Rate limiting on track fetches - implement retry with backoff
		if resp.StatusCode == http.StatusTooManyRequests {
			time.Sleep(2 * time.Second)
			// Retry once
			return c.GetArtistTopTracks(ctx, artistID, limit)
		}
		return nil, fmt.Errorf("get tracks failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tracksResp struct {
		Data []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"data"`
		Included []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"included"`
	}

	if err := json.Unmarshal(body, &tracksResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Extract track IDs from included data (limit to requested number)
	var trackIDs []string
	for _, item := range tracksResp.Included {
		if item.Type == "tracks" {
			trackIDs = append(trackIDs, item.ID)
			if len(trackIDs) >= limit {
				break
			}
		}
	}

	// If no tracks in included, try data section
	if len(trackIDs) == 0 {
		for _, item := range tracksResp.Data {
			if item.Type == "tracks" {
				trackIDs = append(trackIDs, item.ID)
				if len(trackIDs) >= limit {
					break
				}
			}
		}
	}

	return trackIDs, nil
}

// CreateOrUpdatePlaylist creates a new playlist or updates an existing one
func (c *Client) CreateOrUpdatePlaylist(ctx context.Context, name, description string, trackIDs []string) (string, error) {
	if c.accessToken == "" {
		return "", fmt.Errorf("not authenticated")
	}

	// Try to find existing playlist with this name
	existingID, err := c.findPlaylistByName(name)
	if err == nil && existingID != "" {
		// Playlist exists, clear it and update
		fmt.Printf("Found existing playlist, updating...\n")
		if err := c.clearPlaylist(existingID); err != nil {
			return "", fmt.Errorf("clearing playlist: %w", err)
		}

		// Add tracks to existing playlist
		for i := 0; i < len(trackIDs); i += 20 {
			end := i + 20
			if end > len(trackIDs) {
				end = len(trackIDs)
			}
			batch := trackIDs[i:end]

			if err := c.addTracksToPlaylist(existingID, batch); err != nil {
				return existingID, fmt.Errorf("adding tracks: %w", err)
			}
		}

		return existingID, nil
	}

	// Create new playlist
	fmt.Printf("Creating new playlist...\n")
	playlistBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "playlists",
			"attributes": map[string]interface{}{
				"name":        name,
				"description": description,
				"accessType":  "UNLISTED", // Make it unlisted by default
			},
		},
	}

	bodyJSON, err := json.Marshal(playlistBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	// Endpoint: POST /playlists?countryCode=US
	reqURL := fmt.Sprintf("%s/v2/playlists?countryCode=US", apiBaseURL)

	req, err := http.NewRequest("POST", reqURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("Accept", "application/vnd.api+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		// Try to refresh token
		if err := c.RefreshAccessToken(); err != nil {
			return "", fmt.Errorf("token expired and refresh failed: %w", err)
		}
		// Retry with new token
		return c.CreateOrUpdatePlaylist(ctx, name, description, trackIDs)
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("playlist creation failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to get playlist ID
	var playlistResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&playlistResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	playlistID := playlistResp.Data.ID

	// Add tracks to the playlist in batches of 20 (API limit)
	for i := 0; i < len(trackIDs); i += 20 {
		end := i + 20
		if end > len(trackIDs) {
			end = len(trackIDs)
		}
		batch := trackIDs[i:end]

		if err := c.addTracksToPlaylist(playlistID, batch); err != nil {
			return playlistID, fmt.Errorf("adding tracks: %w", err)
		}
	}

	return playlistID, nil
}

// addTracksToPlaylist adds tracks to a playlist
func (c *Client) addTracksToPlaylist(playlistID string, trackIDs []string) error {
	// Build the data array
	var trackData []map[string]interface{}
	for _, trackID := range trackIDs {
		trackData = append(trackData, map[string]interface{}{
			"id":   trackID,
			"type": "tracks",
		})
	}

	requestBody := map[string]interface{}{
		"data": trackData,
	}

	bodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	// Endpoint: POST /playlists/{id}/relationships/items?countryCode=US
	reqURL := fmt.Sprintf("%s/v2/playlists/%s/relationships/items?countryCode=US", apiBaseURL, playlistID)

	req, err := http.NewRequest("POST", reqURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("Accept", "application/vnd.api+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add tracks failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// findPlaylistByName searches for a playlist by name in user's collection
func (c *Client) findPlaylistByName(name string) (string, error) {
	// Get user ID first
	meReq, err := http.NewRequest("GET", apiBaseURL+"/v2/users/me", nil)
	if err != nil {
		return "", fmt.Errorf("creating me request: %w", err)
	}

	meReq.Header.Set("Authorization", "Bearer "+c.accessToken)
	meReq.Header.Set("Accept", "application/vnd.api+json")

	meResp, err := c.httpClient.Do(meReq)
	if err != nil {
		return "", fmt.Errorf("executing me request: %w", err)
	}
	defer meResp.Body.Close()

	if meResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get user info")
	}

	var meData struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(meResp.Body).Decode(&meData); err != nil {
		return "", fmt.Errorf("decoding me response: %w", err)
	}

	userID := meData.Data.ID

	// Get user's playlists
	playlistsURL := fmt.Sprintf("%s/v2/userCollections/%s/relationships/playlists?countryCode=US&include=playlists", apiBaseURL, userID)
	playlistsReq, err := http.NewRequest("GET", playlistsURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating playlists request: %w", err)
	}

	playlistsReq.Header.Set("Authorization", "Bearer "+c.accessToken)
	playlistsReq.Header.Set("Accept", "application/vnd.api+json")

	playlistsResp, err := c.httpClient.Do(playlistsReq)
	if err != nil {
		return "", fmt.Errorf("executing playlists request: %w", err)
	}
	defer playlistsResp.Body.Close()

	if playlistsResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get playlists")
	}

	var playlistsData struct {
		Included []struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"included"`
	}

	if err := json.NewDecoder(playlistsResp.Body).Decode(&playlistsData); err != nil {
		return "", fmt.Errorf("decoding playlists response: %w", err)
	}

	// Find playlist by name
	for _, item := range playlistsData.Included {
		if item.Type == "playlists" && item.Attributes.Name == name {
			return item.ID, nil
		}
	}

	return "", fmt.Errorf("playlist not found")
}

// clearPlaylist removes all tracks from a playlist
func (c *Client) clearPlaylist(playlistID string) error {
	// Get current tracks in the playlist
	getURL := fmt.Sprintf("%s/v2/playlists/%s/relationships/items?countryCode=US&include=tracks", apiBaseURL, playlistID)
	getReq, err := http.NewRequest("GET", getURL, nil)
	if err != nil {
		return fmt.Errorf("creating get request: %w", err)
	}

	getReq.Header.Set("Authorization", "Bearer "+c.accessToken)
	getReq.Header.Set("Accept", "application/vnd.api+json")

	getResp, err := c.httpClient.Do(getReq)
	if err != nil {
		return fmt.Errorf("executing get request: %w", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get playlist items")
	}

	var itemsData struct {
		Data []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Meta struct {
				ItemID string `json:"itemId"`
			} `json:"meta"`
		} `json:"data"`
	}

	if err := json.NewDecoder(getResp.Body).Decode(&itemsData); err != nil {
		return fmt.Errorf("decoding items response: %w", err)
	}

	// If no items, nothing to clear
	if len(itemsData.Data) == 0 {
		return nil
	}

	// Delete all items (need to include meta.itemId for delete)
	var itemsToDelete []map[string]interface{}
	for _, item := range itemsData.Data {
		itemsToDelete = append(itemsToDelete, map[string]interface{}{
			"id":   item.ID,
			"type": item.Type,
			"meta": map[string]interface{}{
				"itemId": item.Meta.ItemID,
			},
		})
	}

	deleteBody := map[string]interface{}{
		"data": itemsToDelete,
	}

	bodyJSON, err := json.Marshal(deleteBody)
	if err != nil {
		return fmt.Errorf("marshaling delete request: %w", err)
	}

	deleteURL := fmt.Sprintf("%s/v2/playlists/%s/relationships/items?countryCode=US", apiBaseURL, playlistID)
	deleteReq, err := http.NewRequest("DELETE", deleteURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return fmt.Errorf("creating delete request: %w", err)
	}

	deleteReq.Header.Set("Authorization", "Bearer "+c.accessToken)
	deleteReq.Header.Set("Content-Type", "application/vnd.api+json")
	deleteReq.Header.Set("Accept", "application/vnd.api+json")

	deleteResp, err := c.httpClient.Do(deleteReq)
	if err != nil {
		return fmt.Errorf("executing delete request: %w", err)
	}
	defer deleteResp.Body.Close()

	if deleteResp.StatusCode != http.StatusNoContent && deleteResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(deleteResp.Body)
		return fmt.Errorf("delete failed with status %d: %s", deleteResp.StatusCode, string(body))
	}

	return nil
}
