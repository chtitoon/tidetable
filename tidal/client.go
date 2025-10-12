package tidal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	apiBaseURL = "https://openapi.tidal.com"
)

// Client provides low-level access to the Tidal API
// All methods are thin wrappers around HTTP requests with minimal logic
type Client struct {
	httpClient *http.Client
	auth       *Authenticator
}

// NewClient creates a new Tidal API client
func NewClient(auth *Authenticator) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		auth: auth,
	}
}

// ===== Generic Request Helper =====

// makeRequest is a generic helper for making authenticated API requests
// Handles authorization header injection and basic error checking
func (c *Client) makeRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Response, error) {
	reqURL := apiBaseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Add authorization header
	req.Header.Set("Authorization", "Bearer "+c.auth.GetAccessToken())
	req.Header.Set("Accept", "application/vnd.api+json")

	if body != nil {
		req.Header.Set("Content-Type", "application/vnd.api+json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	return resp, nil
}

// ===== User API =====

// GetCurrentUser retrieves the authenticated user's information
func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	resp, err := c.makeRequest(ctx, "GET", "/v2/users/me", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get user failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &User{ID: result.Data.ID}, nil
}

// ===== Search API =====

// SearchArtists searches for artists by name
// Returns all matching artists from the search results
func (c *Client) SearchArtists(ctx context.Context, query string) ([]Artist, error) {
	encodedQuery := url.PathEscape(query)
	endpoint := fmt.Sprintf("/v2/searchResults/%s?countryCode=US&include=artists", encodedQuery)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Included []struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			Attributes struct {
				Name       string  `json:"name"`
				Popularity float64 `json:"popularity"`
			} `json:"attributes"`
		} `json:"included"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Extract artists from included data
	var artists []Artist
	for _, item := range result.Included {
		if item.Type == "artists" {
			artists = append(artists, Artist{
				ID:         item.ID,
				Name:       item.Attributes.Name,
				Popularity: item.Attributes.Popularity,
			})
		}
	}

	return artists, nil
}

// ===== Artist Tracks API =====

// GetArtistTracks retrieves tracks for an artist
// Returns track IDs from the artist's catalog
func (c *Client) GetArtistTracks(ctx context.Context, artistID string) ([]string, error) {
	endpoint := fmt.Sprintf("/v2/artists/%s/relationships/tracks?countryCode=US&collapseBy=FINGERPRINT&include=tracks", artistID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get tracks failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"data"`
		Included []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"included"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Extract track IDs from included data first, then data section
	var trackIDs []string
	for _, item := range result.Included {
		if item.Type == "tracks" {
			trackIDs = append(trackIDs, item.ID)
		}
	}

	// If no tracks in included, try data section
	if len(trackIDs) == 0 {
		for _, item := range result.Data {
			if item.Type == "tracks" {
				trackIDs = append(trackIDs, item.ID)
			}
		}
	}

	return trackIDs, nil
}

// ===== Playlist API =====

// GetUserPlaylists retrieves all playlists for a user
func (c *Client) GetUserPlaylists(ctx context.Context, userID string) ([]Playlist, error) {
	endpoint := fmt.Sprintf("/v2/userCollections/%s/relationships/playlists?countryCode=US&include=playlists", userID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get playlists failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Included []struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			Attributes struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"attributes"`
		} `json:"included"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var playlists []Playlist
	for _, item := range result.Included {
		if item.Type == "playlists" {
			playlists = append(playlists, Playlist{
				ID:          item.ID,
				Name:        item.Attributes.Name,
				Description: item.Attributes.Description,
			})
		}
	}

	return playlists, nil
}

// CreatePlaylist creates a new playlist
func (c *Client) CreatePlaylist(ctx context.Context, name, description string) (*Playlist, error) {
	requestBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "playlists",
			"attributes": map[string]interface{}{
				"name":        name,
				"description": description,
				"accessType":  "UNLISTED",
			},
		},
	}

	bodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	endpoint := "/v2/playlists?countryCode=US"
	resp, err := c.makeRequest(ctx, "POST", endpoint, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create playlist failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"attributes"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &Playlist{
		ID:          result.Data.ID,
		Name:        result.Data.Attributes.Name,
		Description: result.Data.Attributes.Description,
	}, nil
}

// GetPlaylistItems retrieves all items (tracks) in a playlist
func (c *Client) GetPlaylistItems(ctx context.Context, playlistID string) ([]PlaylistItem, error) {
	endpoint := fmt.Sprintf("/v2/playlists/%s/relationships/items?countryCode=US&include=tracks", playlistID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get playlist items failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Meta struct {
				ItemID string `json:"itemId"`
			} `json:"meta"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var items []PlaylistItem
	for _, item := range result.Data {
		items = append(items, PlaylistItem{
			ID:     item.ID,
			Type:   item.Type,
			ItemID: item.Meta.ItemID,
		})
	}

	return items, nil
}

// AddTracksToPlaylist adds tracks to a playlist
func (c *Client) AddTracksToPlaylist(ctx context.Context, playlistID string, trackIDs []string) error {
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

	endpoint := fmt.Sprintf("/v2/playlists/%s/relationships/items?countryCode=US", playlistID)
	resp, err := c.makeRequest(ctx, "POST", endpoint, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add tracks failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DeletePlaylistItems removes items from a playlist
func (c *Client) DeletePlaylistItems(ctx context.Context, playlistID string, items []PlaylistItem) error {
	var itemsToDelete []map[string]interface{}
	for _, item := range items {
		itemsToDelete = append(itemsToDelete, map[string]interface{}{
			"id":   item.ID,
			"type": item.Type,
			"meta": map[string]interface{}{
				"itemId": item.ItemID,
			},
		})
	}

	deleteBody := map[string]interface{}{
		"data": itemsToDelete,
	}

	bodyJSON, err := json.Marshal(deleteBody)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	endpoint := fmt.Sprintf("/v2/playlists/%s/relationships/items?countryCode=US", playlistID)
	resp, err := c.makeRequest(ctx, "DELETE", endpoint, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete items failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
