package tidal

import "time"

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

// ===== JSON:API documents (artist export) =====

// ResourceIdentifier is a JSON:API resource linkage ({id, type}). In user
// collections, meta.addedAt records when the item was liked or followed.
type ResourceIdentifier struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Meta *struct {
		AddedAt string `json:"addedAt"`
	} `json:"meta,omitempty"`
}

// AddedAt returns when a collection item was added, or false if unknown.
func (r ResourceIdentifier) AddedAt() (time.Time, bool) {
	if r.Meta == nil || r.Meta.AddedAt == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, r.Meta.AddedAt)
	return t, err == nil
}

// Resource is an included JSON:API resource. Only the fields the artist
// export needs are decoded: artists carry a name, albums and tracks a title,
// and albums/tracks list their artists in credit order (main artist first).
type Resource struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Attributes struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	} `json:"attributes"`
	Relationships struct {
		Artists *struct {
			Data []ResourceIdentifier `json:"data"`
		} `json:"artists"`
	} `json:"relationships"`
}

// ArtistIDs returns the resource's artist linkage in credit order.
func (r Resource) ArtistIDs() []string {
	if r.Relationships.Artists == nil {
		return nil
	}
	ids := make([]string, 0, len(r.Relationships.Artists.Data))
	for _, a := range r.Relationships.Artists.Data {
		ids = append(ids, a.ID)
	}
	return ids
}

// Page is one page of a to-many relationship read.
type Page struct {
	Data     []ResourceIdentifier `json:"data"`
	Included []Resource           `json:"included"`
	Links    struct {
		Next string `json:"next"`
	} `json:"links"`
}

// Collection is a fully paginated user collection: the items in API order
// plus every included resource, keyed by "type:id".
type Collection struct {
	Items    []ResourceIdentifier
	Included map[string]Resource
}

// Lookup returns the included resource for an identifier.
func (c Collection) Lookup(id ResourceIdentifier) (Resource, bool) {
	r, ok := c.Included[id.Type+":"+id.ID]
	return r, ok
}
