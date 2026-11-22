// Package artistexport turns a Tidal library into the scored data/artists.json
// consumed by the weekly concert check-in.
package artistexport

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/chtitoon/tidetable/tidal"
)

// Scoring weights. Only the main (first-credited) artist of an album or track scores.
const (
	PointsFollowed   = 3
	PointsLikedAlbum = 2
	PointsLikedTrack = 1

	// DefaultThreshold is written into the file; similar artists are fetched
	// only for artists at or above it. Filtering otherwise happens downstream.
	DefaultThreshold = 2
)

// nonArtists are credits that are not real performers (compilations, labels,
// Tidal playlist accounts). They never score, so a compilation credits nobody.
// Matched case-insensitively.
var nonArtists = map[string]bool{
	"various artists":     true,
	"war child records":   true,
	"indie/rock playlist": true,
}

// IsNonArtist reports whether a credit name is a compilation or label placeholder.
func IsNonArtist(name string) bool {
	return nonArtists[strings.ToLower(strings.TrimSpace(name))]
}

// ===== Output shape =====

type Export struct {
	GeneratedAt       string             `json:"generated_at"`
	Threshold         int                `json:"threshold"`
	Artists           []Artist           `json:"artists"`
	SimilarCandidates []SimilarCandidate `json:"similar_candidates"`
}

type Artist struct {
	TidalID string       `json:"tidal_id"`
	Name    string       `json:"name"`
	Score   int          `json:"score"`
	Signals Signals      `json:"signals"`
	Similar []SimilarRef `json:"similar"`
}

type Signals struct {
	Followed    bool `json:"followed"`
	LikedAlbums int  `json:"liked_albums"`
	LikedTracks int  `json:"liked_tracks"`
}

type SimilarRef struct {
	TidalID   string `json:"tidal_id"`
	Name      string `json:"name"`
	InLibrary bool   `json:"in_library"`
}

// SimilarCandidate aggregates similar artists that are NOT in the library.
// Weight = how many library artists list them as similar (primary sort key);
// SourceScore = sum of those library artists' scores (tie-breaker).
type SimilarCandidate struct {
	TidalID     string   `json:"tidal_id"`
	Name        string   `json:"name"`
	Weight      int      `json:"weight"`
	SourceScore int      `json:"source_score"`
	Sources     []string `json:"sources"`
}

func (a Artist) score() int {
	s := a.Signals.LikedAlbums*PointsLikedAlbum + a.Signals.LikedTracks*PointsLikedTrack
	if a.Signals.Followed {
		s += PointsFollowed
	}
	return s
}

// ===== Scoring =====

// Stats summarises what scoring saw, for logging.
type Stats struct {
	Followed, Albums, Tracks int
	NoArtist                 int // albums/tracks without artist linkage (e.g. unavailable)
	NonArtistMain            int // albums/tracks whose main credit is a compilation/label
	// Unresolved lists artist ids credited (or followed) but missing from the
	// included resources, e.g. an artist page Tidal has since removed (404).
	// Their credits are skipped.
	Unresolved []string
}

// MainArtistID returns the main artist of an album or track: the first credit.
// Tidal exposes no per-credit role, but lists the primary artist first and
// featured artists after it.
func MainArtistID(r tidal.Resource) (string, bool) {
	ids := r.ArtistIDs()
	if len(ids) == 0 {
		return "", false
	}
	return ids[0], true
}

// Score builds the scored artist list (without similar artists) from the
// three collections, sorted by score desc then name then id.
func Score(followed, albums, tracks tidal.Collection) ([]Artist, Stats) {
	names := map[string]string{}
	for _, col := range []tidal.Collection{followed, albums, tracks} {
		for _, r := range col.Included {
			if r.Type == "artists" && r.Attributes.Name != "" {
				names[r.ID] = r.Attributes.Name
			}
		}
	}

	byID := map[string]*Artist{}
	get := func(id string) *Artist {
		if a, ok := byID[id]; ok {
			return a
		}
		a := &Artist{TidalID: id, Name: names[id], Similar: []SimilarRef{}}
		byID[id] = a
		return a
	}

	stats := Stats{Followed: len(followed.Items), Albums: len(albums.Items), Tracks: len(tracks.Items)}

	for _, it := range followed.Items {
		if it.Type != "artists" || IsNonArtist(names[it.ID]) {
			continue
		}
		if names[it.ID] == "" {
			stats.Unresolved = append(stats.Unresolved, it.ID)
			continue
		}
		get(it.ID).Signals.Followed = true
	}

	credit := func(col tidal.Collection, wantType string, bump func(*Artist)) {
		for _, it := range col.Items {
			if it.Type != wantType {
				continue
			}
			r, ok := col.Lookup(it)
			if !ok {
				stats.NoArtist++
				continue
			}
			main, ok := MainArtistID(r)
			if !ok {
				stats.NoArtist++
				continue
			}
			if names[main] == "" {
				stats.Unresolved = append(stats.Unresolved, main)
				continue
			}
			if IsNonArtist(names[main]) {
				stats.NonArtistMain++
				continue
			}
			bump(get(main))
		}
	}
	credit(albums, "albums", func(a *Artist) { a.Signals.LikedAlbums++ })
	credit(tracks, "tracks", func(a *Artist) { a.Signals.LikedTracks++ })

	out := make([]Artist, 0, len(byID))
	for _, a := range byID {
		a.Score = a.score()
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].TidalID < out[j].TidalID
	})
	return out, stats
}

// ===== Assembly =====

// Build attaches similar artists (keyed by library artist id) and computes
// the aggregated candidate list. artists must come from Score.
func Build(artists []Artist, similar map[string][]tidal.Artist, threshold int, now time.Time) Export {
	inLibrary := make(map[string]bool, len(artists))
	for _, a := range artists {
		inLibrary[a.TidalID] = true
	}

	cands := map[string]*SimilarCandidate{}

	out := make([]Artist, len(artists))
	for i, a := range artists {
		a.Similar = []SimilarRef{}
		seen := map[string]bool{}
		for _, s := range similar[a.TidalID] {
			if s.ID == a.TidalID || seen[s.ID] || s.Name == "" || IsNonArtist(s.Name) {
				continue
			}
			seen[s.ID] = true
			a.Similar = append(a.Similar, SimilarRef{TidalID: s.ID, Name: s.Name, InLibrary: inLibrary[s.ID]})

			if inLibrary[s.ID] {
				continue
			}
			c, ok := cands[s.ID]
			if !ok {
				c = &SimilarCandidate{TidalID: s.ID, Name: s.Name}
				cands[s.ID] = c
			}
			// seen[] guarantees each library artist counts once per candidate.
			c.Weight++
			c.SourceScore += a.Score
			c.Sources = append(c.Sources, a.Name)
		}
		sort.Slice(a.Similar, func(x, y int) bool {
			if a.Similar[x].Name != a.Similar[y].Name {
				return a.Similar[x].Name < a.Similar[y].Name
			}
			return a.Similar[x].TidalID < a.Similar[y].TidalID
		})
		out[i] = a
	}

	candidates := make([]SimilarCandidate, 0, len(cands))
	for _, c := range cands {
		sort.Strings(c.Sources)
		candidates = append(candidates, *c)
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Weight != b.Weight {
			return a.Weight > b.Weight
		}
		if a.SourceScore != b.SourceScore {
			return a.SourceScore > b.SourceScore
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.TidalID < b.TidalID
	})

	return Export{
		GeneratedAt:       now.UTC().Format(time.RFC3339),
		Threshold:         threshold,
		Artists:           out,
		SimilarCandidates: candidates,
	}
}

// ===== Validation =====

// Validate checks internal consistency before anything is written.
func (e Export) Validate() error {
	if _, err := time.Parse(time.RFC3339, e.GeneratedAt); err != nil {
		return fmt.Errorf("generated_at %q is not RFC 3339: %w", e.GeneratedAt, err)
	}
	if e.Threshold < 1 {
		return fmt.Errorf("threshold must be >= 1, got %d", e.Threshold)
	}
	if len(e.Artists) == 0 {
		return fmt.Errorf("no artists")
	}
	ids := map[string]bool{}
	for i, a := range e.Artists {
		if a.TidalID == "" || a.Name == "" {
			return fmt.Errorf("artist #%d has an empty id or name", i)
		}
		if ids[a.TidalID] {
			return fmt.Errorf("duplicate artist %s (%s)", a.TidalID, a.Name)
		}
		ids[a.TidalID] = true
		if a.Score != a.score() || a.Score <= 0 {
			return fmt.Errorf("artist %s has score %d inconsistent with signals %+v", a.Name, a.Score, a.Signals)
		}
		if i > 0 && e.Artists[i-1].Score < a.Score {
			return fmt.Errorf("artists not sorted by score at %s", a.Name)
		}
		if len(a.Similar) > 0 && a.Score < e.Threshold {
			return fmt.Errorf("artist %s below threshold has similar artists", a.Name)
		}
	}
	for _, a := range e.Artists {
		for _, s := range a.Similar {
			if s.InLibrary != ids[s.TidalID] {
				return fmt.Errorf("similar %s under %s has wrong in_library", s.Name, a.Name)
			}
		}
	}
	for _, c := range e.SimilarCandidates {
		if ids[c.TidalID] {
			return fmt.Errorf("candidate %s is already in the library", c.Name)
		}
		if c.Weight != len(c.Sources) || c.Weight == 0 {
			return fmt.Errorf("candidate %s weight %d != %d sources", c.Name, c.Weight, len(c.Sources))
		}
	}
	return nil
}
