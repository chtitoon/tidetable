// Package artistexport turns a Tidal library into the ranked data/artists.json
// consumed by the weekly concert check-in and the website.
package artistexport

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/chtitoon/tidetable/tidal"
)

// Scoring. Scores only rank the output; they are not written to the file.
// Only the main (first-credited) artist of an album or track scores.
//
// Each artist gets two scores from the same signals:
//
//	score = follow + WeightAlbums·ln(1+Σ album weights) + WeightTracks·ln(1+Σ track weights)
//
// The log damps piles of likes: 30 hearted tracks don't outweigh a follow and
// a few albums.
//
//   - All-time (my_artists): every item counts in full, whatever its age.
//     Recency only breaks ties; recent_artists covers "lately".
//   - Lately (recent_artists): only items from the last RecentWindow count,
//     decaying with RecentHalfLife, follows included.
//
// Tidal exposes no listening history, so recent likes stand in for
// "listening to them a lot lately".
const (
	PointsFollowed = 4.0 // about a dozen hearted tracks, or two to three albums
	WeightAlbums   = 3.0
	WeightTracks   = 1.5

	RecentWindow   = 90 * 24 * time.Hour
	RecentHalfLife = 45 * 24 * time.Hour
	RecentMinItems = 2 // follows/likes in the window to be listed in recent_artists

	// SimilarThreshold: similar artists are fetched only for library artists
	// with at least this all-time score (a follow, or an album and a track).
	SimilarThreshold = 3.0

	// PopularityPenalty scales similar artists down by Tidal popularity (0..1)
	// to favour lesser-known ones: a fully popular artist keeps a quarter.
	PopularityPenalty = 0.75
)

// TimeRange is a half-open interval [From, To).
type TimeRange struct{ From, To time.Time }

func (r TimeRange) Contains(t time.Time) bool { return !t.Before(r.From) && t.Before(r.To) }

// RecentIgnoredPeriods are periods whose album likes and follows count for
// my_artists but not for recent_artists: bulk likes/follows made to correct
// the all-time list, not because of what was being listened to lately.
// Hearted tracks are unaffected. Entries can be removed once they fall
// outside RecentWindow.
var RecentIgnoredPeriods = []TimeRange{
	// 2026-11-21 evening (EST): liked older albums and followed artists to
	// course-correct my_artists.
	{time.Date(2026, 11, 22, 2, 15, 0, 0, time.UTC), time.Date(2026, 11, 22, 4, 30, 0, 0, time.UTC)},
}

func inAny(ranges []TimeRange, t time.Time) bool {
	for _, r := range ranges {
		if r.Contains(t) {
			return true
		}
	}
	return false
}

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
	return nonArtists[normName(name)]
}

func normName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// ===== Output shape =====

// Export is the file format. All lists are ranked, most relevant first.
type Export struct {
	GeneratedAt    string `json:"generated_at"`
	MyArtists      []Ref  `json:"my_artists"`      // all-time
	RecentArtists  []Ref  `json:"recent_artists"`  // lately (last 90 days), a subset of my_artists
	SimilarArtists []Ref  `json:"similar_artists"` // not in the library
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
	// AltIDs are other Tidal profiles with the same name, merged into this
	// one (Tidal has duplicates, e.g. two "Sufjan Stevens"). TidalID is the
	// profile with the most credits.
	AltIDs  []string
	Score   float64 // all-time
	Recent  float64 // lately; 0 unless Signals.RecentItems >= RecentMinItems
	Signals Signals
	// AllTimeParts and RecentParts break Score and Recent down, for -explain.
	AllTimeParts, RecentParts Parts
}

// Parts are the points each signal contributes to a score.
type Parts struct {
	Follow, Albums, Tracks float64
}

func (p Parts) Total() float64 { return p.Follow + p.Albums + p.Tracks }

func parts(follow, albums, tracks float64) Parts {
	return Parts{Follow: follow, Albums: WeightAlbums * math.Log1p(albums), Tracks: WeightTracks * math.Log1p(tracks)}
}

type Signals struct {
	Followed    bool
	LikedAlbums int
	LikedTracks int
	RecentItems int       // follows/likes within RecentWindow
	LatestAdded time.Time // most recent follow/like, tie-breaker
}

// Stats summarises what scoring saw, for logging.
type Stats struct {
	Followed, Albums, Tracks int
	NoArtist                 int // albums/tracks without artist linkage (e.g. unavailable)
	NonArtistMain            int // albums/tracks whose main credit is a compilation/label
	Merged                   int // duplicate artist profiles merged by name
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

// decay is 0.5^(age/halfLife); items dated in the future count in full.
func decay(age, halfLife time.Duration) float64 {
	if age <= 0 {
		return 1
	}
	return math.Pow(0.5, float64(age)/float64(halfLife))
}

// recentWeight weighs an item for recent_artists: 0 if undated or outside RecentWindow.
func recentWeight(at time.Time, dated bool, now time.Time) float64 {
	if !dated || now.Sub(at) > RecentWindow {
		return 0
	}
	return decay(now.Sub(at), RecentHalfLife)
}

// tally accumulates one artist's weighted signals before they are combined.
type tally struct {
	artist                     Artist
	credits                    map[string]int // per Tidal profile, to pick the main one
	followRecent               float64
	albumsAll, tracksAll       float64
	albumsRecent, tracksRecent float64
}

// Score ranks library artists from the three collections, all-time: by score
// desc, then most recent like/follow, then name, then id. Profiles sharing a
// name are merged into one artist. RankRecent gives the lately ranking.
func Score(followed, albums, tracks tidal.Collection, now time.Time) ([]Artist, Stats) {
	names := map[string]string{}
	for _, col := range []tidal.Collection{followed, albums, tracks} {
		for _, r := range col.Included {
			if r.Type == "artists" && r.Attributes.Name != "" {
				names[r.ID] = r.Attributes.Name
			}
		}
	}

	byName := map[string]*tally{}
	stats := Stats{Followed: len(followed.Items), Albums: len(albums.Items), Tracks: len(tracks.Items)}

	// add credits an item to an artist; signal records it with its all-time
	// weight (always 1) and recent weight. Items added during one of
	// ignoreRecent get no recent weight.
	add := func(artistID string, item tidal.ResourceIdentifier, ignoreRecent []TimeRange, signal func(t *tally, all, recent float64)) {
		name := names[artistID]
		if name == "" {
			stats.Unresolved = append(stats.Unresolved, artistID)
			return
		}
		key := normName(name)
		t, ok := byName[key]
		if !ok {
			t = &tally{credits: map[string]int{}}
			byName[key] = t
		}
		t.credits[artistID]++
		at, dated := item.AddedAt()
		all, recent := 1.0, recentWeight(at, dated, now)
		if dated && inAny(ignoreRecent, at) {
			recent = 0
		}
		if recent > 0 {
			t.artist.Signals.RecentItems++
		}
		if dated && at.After(t.artist.Signals.LatestAdded) {
			t.artist.Signals.LatestAdded = at
		}
		signal(t, all, recent)
	}

	for _, it := range followed.Items {
		if it.Type != "artists" || IsNonArtist(names[it.ID]) {
			continue
		}
		add(it.ID, it, RecentIgnoredPeriods, func(t *tally, _, recent float64) {
			t.artist.Signals.Followed = true // counts once, even if two profiles are followed
			t.followRecent = max(t.followRecent, recent)
		})
	}

	credit := func(col tidal.Collection, wantType string, ignoreRecent []TimeRange, signal func(t *tally, all, recent float64)) {
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
			add(main, it, ignoreRecent, signal)
		}
	}
	credit(albums, "albums", RecentIgnoredPeriods, func(t *tally, all, recent float64) {
		t.artist.Signals.LikedAlbums++
		t.albumsAll += all
		t.albumsRecent += recent
	})
	credit(tracks, "tracks", nil, func(t *tally, all, recent float64) {
		t.artist.Signals.LikedTracks++
		t.tracksAll += all
		t.tracksRecent += recent
	})

	out := make([]Artist, 0, len(byName))
	for _, t := range byName {
		a := t.artist
		ids := make([]string, 0, len(t.credits))
		for id := range t.credits {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool {
			if t.credits[ids[i]] != t.credits[ids[j]] {
				return t.credits[ids[i]] > t.credits[ids[j]]
			}
			return ids[i] < ids[j]
		})
		a.TidalID, a.AltIDs = ids[0], ids[1:]
		a.Name = names[a.TidalID]
		stats.Merged += len(a.AltIDs)

		follow := 0.0
		if a.Signals.Followed {
			follow = PointsFollowed
		}
		a.AllTimeParts = parts(follow, t.albumsAll, t.tracksAll)
		a.Score = a.AllTimeParts.Total()
		if a.Signals.RecentItems >= RecentMinItems {
			a.RecentParts = parts(PointsFollowed*t.followRecent, t.albumsRecent, t.tracksRecent)
			a.Recent = a.RecentParts.Total()
		}
		out = append(out, a)
	}
	sortArtists(out, func(a Artist) float64 { return a.Score })
	return out, stats
}

// RankRecent returns the artists that have a lately score, ranked by it.
func RankRecent(artists []Artist) []Artist {
	out := []Artist{}
	for _, a := range artists {
		if a.Recent > 0 {
			out = append(out, a)
		}
	}
	sortArtists(out, func(a Artist) float64 { return a.Recent })
	return out
}

// sortArtists orders by score desc, then most recent like/follow, name, id.
func sortArtists(artists []Artist, score func(Artist) float64) {
	sort.Slice(artists, func(i, j int) bool {
		a, b := artists[i], artists[j]
		if sa, sb := score(a), score(b); sa != sb {
			return sa > sb
		}
		if !a.Signals.LatestAdded.Equal(b.Signals.LatestAdded) {
			return a.Signals.LatestAdded.After(b.Signals.LatestAdded)
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.TidalID < b.TidalID
	})
}

// ===== Assembly =====

// Build assembles the export. artists must come from Score; similar is keyed
// by library artist id.
//
// Similar artists not in the library are ranked by the summed all-time score
// of the library artists listing them as similar, scaled down by their Tidal
// popularity (PopularityPenalty) to favour discovery, then by how many library
// artists list them, then name.
//
// A similar artist counts as "in the library" if its id OR its name matches a
// library artist: Tidal has duplicate profiles for some artists (e.g. two
// "Sufjan Stevens" ids), and the check-in matches concerts by name anyway.
func Build(artists []Artist, similar map[string][]tidal.Artist, now time.Time) Export {
	inLibrary := make(map[string]bool, len(artists))
	libraryNames := make(map[string]bool, len(artists))
	mine := make([]Ref, len(artists))
	for i, a := range artists {
		inLibrary[a.TidalID] = true
		for _, id := range a.AltIDs {
			inLibrary[id] = true
		}
		libraryNames[normName(a.Name)] = true
		mine[i] = Ref{TidalID: a.TidalID, Name: a.Name}
	}
	for _, list := range similar {
		for _, s := range list {
			if libraryNames[normName(s.Name)] {
				inLibrary[s.ID] = true
			}
		}
	}

	recentArtists := RankRecent(artists)
	recent := make([]Ref, len(recentArtists))
	for i, a := range recentArtists {
		recent[i] = Ref{TidalID: a.TidalID, Name: a.Name}
	}

	type cand struct {
		ref         Ref
		crossRefs   int
		sourceScore float64
		popularity  float64
		score       float64
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
			c.popularity = max(c.popularity, min(max(s.Popularity, 0), 1))
		}
	}

	ranked := make([]*cand, 0, len(cands))
	for _, c := range cands {
		c.score = c.sourceScore * (1 - PopularityPenalty*c.popularity)
		ranked = append(ranked, c)
	}
	sort.Slice(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.crossRefs != b.crossRefs {
			return a.crossRefs > b.crossRefs
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
		RecentArtists:  recent,
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
		if mine["name:"+normName(a.Name)] {
			return fmt.Errorf("duplicate artist name %s in my_artists", a.Name)
		}
		mine[a.TidalID] = true
		mine["name:"+normName(a.Name)] = true
	}
	if e.RecentArtists == nil {
		return fmt.Errorf("recent_artists is null, want a list")
	}
	seenRecent := map[string]bool{}
	for i, r := range e.RecentArtists {
		if r.TidalID == "" || r.Name == "" {
			return fmt.Errorf("recent_artists #%d has an empty id or name", i)
		}
		if !mine[r.TidalID] {
			return fmt.Errorf("recent artist %s is not in my_artists", r.Name)
		}
		if seenRecent[r.TidalID] {
			return fmt.Errorf("duplicate artist %s (%s) in recent_artists", r.TidalID, r.Name)
		}
		seenRecent[r.TidalID] = true
	}
	seen := map[string]bool{}
	for i, s := range e.SimilarArtists {
		if s.TidalID == "" || s.Name == "" {
			return fmt.Errorf("similar_artists #%d has an empty id or name", i)
		}
		if mine[s.TidalID] || mine["name:"+normName(s.Name)] {
			return fmt.Errorf("similar artist %s is already in my_artists", s.Name)
		}
		if seen[s.TidalID] {
			return fmt.Errorf("duplicate artist %s (%s) in similar_artists", s.TidalID, s.Name)
		}
		seen[s.TidalID] = true
	}
	return nil
}
