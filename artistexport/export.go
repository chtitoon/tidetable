// Package artistexport turns a Tidal library into the ranked data/artists.json
// consumed by the weekly concert check-in.
package artistexport

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/chtitoon/tidetable/tidal"
)

// Scoring. Scores only rank the output; they are not written to the file.
// Only the main (first-credited) artist of an album or track scores.
const (
	PointsFollowed   = 3
	PointsLikedAlbum = 2
	PointsLikedTrack = 1

	// Items liked or followed within RecentWindow count RecentMultiplier times.
	// Tidal exposes no listening history, so recent likes stand in for
	// "listening to them a lot lately".
	RecentWindow     = 183 * 24 * time.Hour // ~6 months
	RecentMultiplier = 2

	// SimilarThreshold: similar artists are fetched only for library artists
	// with at least this score.
	SimilarThreshold = 2
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

// Export is the file format. Both lists are ranked, most relevant first.
type Export struct {
	GeneratedAt    string `json:"generated_at"`
	MyArtists      []Ref  `json:"my_artists"`
	SimilarArtists []Ref  `json:"similar_artists"`
}

type Ref struct {
	TidalID string `json:"tidal_id"`
	Name    string `json:"name"`
}

// ===== Scoring =====

// Artist is a scored library artist (internal; only TidalID and Name are exported).
type Artist struct {
	TidalID string
	Name    string
	Score   int
	Signals Signals
}

type Signals struct {
	Followed    bool
	LikedAlbums int
	LikedTracks int
	RecentItems int       // follows/likes within RecentWindow (already in Score)
	LatestAdded time.Time // most recent follow/like, tie-breaker
}

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

// Score ranks library artists from the three collections: by score desc,
// then most recent like/follow, then name, then id.
func Score(followed, albums, tracks tidal.Collection, now time.Time) ([]Artist, Stats) {
	names := map[string]string{}
	for _, col := range []tidal.Collection{followed, albums, tracks} {
		for _, r := range col.Included {
			if r.Type == "artists" && r.Attributes.Name != "" {
				names[r.ID] = r.Attributes.Name
			}
		}
	}

	byID := map[string]*Artist{}
	stats := Stats{Followed: len(followed.Items), Albums: len(albums.Items), Tracks: len(tracks.Items)}

	// add credits points to an artist, multiplied if the item was added recently.
	add := func(artistID string, item tidal.ResourceIdentifier, points int, signal func(*Signals)) {
		if names[artistID] == "" {
			stats.Unresolved = append(stats.Unresolved, artistID)
			return
		}
		a, ok := byID[artistID]
		if !ok {
			a = &Artist{TidalID: artistID, Name: names[artistID]}
			byID[artistID] = a
		}
		signal(&a.Signals)
		if at, ok := item.AddedAt(); ok {
			if now.Sub(at) <= RecentWindow {
				points *= RecentMultiplier
				a.Signals.RecentItems++
			}
			if at.After(a.Signals.LatestAdded) {
				a.Signals.LatestAdded = at
			}
		}
		a.Score += points
	}

	for _, it := range followed.Items {
		if it.Type != "artists" || IsNonArtist(names[it.ID]) {
			continue
		}
		add(it.ID, it, PointsFollowed, func(s *Signals) { s.Followed = true })
	}

	credit := func(col tidal.Collection, wantType string, points int, signal func(*Signals)) {
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
			if IsNonArtist(names[main]) {
				stats.NonArtistMain++
				continue
			}
			add(main, it, points, signal)
		}
	}
	credit(albums, "albums", PointsLikedAlbum, func(s *Signals) { s.LikedAlbums++ })
	credit(tracks, "tracks", PointsLikedTrack, func(s *Signals) { s.LikedTracks++ })

	out := make([]Artist, 0, len(byID))
	for _, a := range byID {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if !a.Signals.LatestAdded.Equal(b.Signals.LatestAdded) {
			return a.Signals.LatestAdded.After(b.Signals.LatestAdded)
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.TidalID < b.TidalID
	})
	return out, stats
}

// ===== Assembly =====

// Build ranks similar artists that are not in the library by how many library
// artists list them as similar (cross-references), then by the summed score of
// those library artists, then name. similar is keyed by library artist id;
// artists must come from Score.
func Build(artists []Artist, similar map[string][]tidal.Artist, now time.Time) Export {
	inLibrary := make(map[string]bool, len(artists))
	mine := make([]Ref, len(artists))
	for i, a := range artists {
		inLibrary[a.TidalID] = true
		mine[i] = Ref{TidalID: a.TidalID, Name: a.Name}
	}

	type cand struct {
		ref         Ref
		crossRefs   int
		sourceScore int
	}
	cands := map[string]*cand{}
	for _, a := range artists {
		seen := map[string]bool{}
		for _, s := range similar[a.TidalID] {
			if inLibrary[s.ID] || seen[s.ID] || s.Name == "" || IsNonArtist(s.Name) {
				continue
			}
			seen[s.ID] = true // each library artist counts once per candidate
			c, ok := cands[s.ID]
			if !ok {
				c = &cand{ref: Ref{TidalID: s.ID, Name: s.Name}}
				cands[s.ID] = c
			}
			c.crossRefs++
			c.sourceScore += a.Score
		}
	}

	ranked := make([]*cand, 0, len(cands))
	for _, c := range cands {
		ranked = append(ranked, c)
	}
	sort.Slice(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.crossRefs != b.crossRefs {
			return a.crossRefs > b.crossRefs
		}
		if a.sourceScore != b.sourceScore {
			return a.sourceScore > b.sourceScore
		}
		if a.ref.Name != b.ref.Name {
			return a.ref.Name < b.ref.Name
		}
		return a.ref.TidalID < b.ref.TidalID
	})
	sim := make([]Ref, len(ranked))
	for i, c := range ranked {
		sim[i] = c.ref
	}

	return Export{
		GeneratedAt:    now.UTC().Format(time.RFC3339),
		MyArtists:      mine,
		SimilarArtists: sim,
	}
}

// ===== Validation =====

// Validate checks internal consistency before anything is written.
func (e Export) Validate() error {
	if _, err := time.Parse(time.RFC3339, e.GeneratedAt); err != nil {
		return fmt.Errorf("generated_at %q is not RFC 3339: %w", e.GeneratedAt, err)
	}
	if len(e.MyArtists) == 0 {
		return fmt.Errorf("no artists")
	}
	mine := map[string]bool{}
	for i, a := range e.MyArtists {
		if a.TidalID == "" || a.Name == "" {
			return fmt.Errorf("my_artists #%d has an empty id or name", i)
		}
		if mine[a.TidalID] {
			return fmt.Errorf("duplicate artist %s (%s) in my_artists", a.TidalID, a.Name)
		}
		mine[a.TidalID] = true
	}
	seen := map[string]bool{}
	for i, s := range e.SimilarArtists {
		if s.TidalID == "" || s.Name == "" {
			return fmt.Errorf("similar_artists #%d has an empty id or name", i)
		}
		if mine[s.TidalID] {
			return fmt.Errorf("similar artist %s is already in my_artists", s.Name)
		}
		if seen[s.TidalID] {
			return fmt.Errorf("duplicate artist %s (%s) in similar_artists", s.TidalID, s.Name)
		}
		seen[s.TidalID] = true
	}
	return nil
}
