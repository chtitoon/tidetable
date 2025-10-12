package tidal

// Artist represents a Tidal artist
type Artist struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Popularity float64 `json:"popularity,omitempty"`
}

// Track represents a Tidal track
type Track struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Playlist represents a Tidal playlist
type Playlist struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PlaylistItem represents an item in a playlist (includes metadata for deletion)
type PlaylistItem struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	ItemID string `json:"itemId"` // Meta field needed for deletion
}

// User represents a Tidal user
type User struct {
	ID string `json:"id"`
}

// tokenResponse represents OAuth token response
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}
