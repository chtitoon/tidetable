package artistexport

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chtitoon/tidetable/tidal"
)

// fixtureNow is "today" for the fixtures.
var fixtureNow = time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)

// loadFixture reads a recorded JSON:API page (trimmed real responses from
// /v2/userCollection*/me/relationships/items, with meta.addedAt) into a Collection.
func loadFixture(t *testing.T, name string) tidal.Collection {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return pageToCollection(t, raw)
}

func pageToCollection(t *testing.T, raw []byte) tidal.Collection {
	t.Helper()
	var p tidal.Page
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	col := tidal.Collection{Items: p.Data, Included: map[string]tidal.Resource{}}
	for _, r := range p.Included {
		col.Included[r.Type+":"+r.ID] = r
	}
	return col
}

// ===== Synthetic libraries =====

type credit struct{ id, name string }

// item is one collection entry: a followed artist (artists[0]) or a liked
// album/track crediting artists in order.
type item struct {
	id      string
	added   time.Time
	artists []credit
}

// synth builds a collection of type typ ("artists", "albums" or "tracks")
// through the same JSON decoding as the real API.
func synth(t *testing.T, typ string, items ...item) tidal.Collection {
	t.Helper()
	type rid struct {
		ID   string         `json:"id"`
		Type string         `json:"type"`
		Meta map[string]any `json:"meta,omitempty"`
	}
	var data []rid
	var included []map[string]any
	artistRes := func(c credit) map[string]any {
		return map[string]any{"id": c.id, "type": "artists", "attributes": map[string]any{"name": c.name}}
	}
	for _, it := range items {
		meta := map[string]any{"addedAt": it.added.Format(time.RFC3339)}
		if typ == "artists" {
			data = append(data, rid{ID: it.artists[0].id, Type: "artists", Meta: meta})
			included = append(included, artistRes(it.artists[0]))
			continue
		}
		data = append(data, rid{ID: it.id, Type: typ, Meta: meta})
		var links []rid
		for _, c := range it.artists {
			links = append(links, rid{ID: c.id, Type: "artists"})
			included = append(included, artistRes(c))
		}
		included = append(included, map[string]any{
			"id": it.id, "type": typ, "attributes": map[string]any{"title": it.id},
			"relationships": map[string]any{"artists": map[string]any{"data": links}},
		})
	}
	raw, err := json.Marshal(map[string]any{"data": data, "included": included})
	if err != nil {
		t.Fatal(err)
	}
	return pageToCollection(t, raw)
}

func daysAgo(d int) time.Time { return fixtureNow.Add(-time.Duration(d) * 24 * time.Hour) }

func manyItems(prefix string, n, ageDays int, a credit) []item {
	out := make([]item, n)
	for i := range out {
		out[i] = item{id: prefix + string(rune('a'+i)), added: daysAgo(ageDays), artists: []credit{a}}
	}
	return out
}

func scoreFixtures(t *testing.T) ([]Artist, Stats) {
	t.Helper()
	return Score(loadFixture(t, "followed.json"), loadFixture(t, "albums.json"), loadFixture(t, "tracks.json"), fixtureNow)
}

func byName(artists []Artist) map[string]Artist {
	m := map[string]Artist{}
	for _, a := range artists {
		m[a.Name] = a
	}
	return m
}

func artistNames(artists []Artist) []string {
	out := make([]string, len(artists))
	for i, a := range artists {
		out[i] = a.Name
	}
	return out
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// ===== Score =====

func TestScore(t *testing.T) {
	artists, stats := scoreFixtures(t)

	// The National: follow + album (2023). Gorillaz: follow (2021) + track.
	// Big Thief: follow only. Neil Young: one album liked two weeks ago, the
	// highest non-follower. Then single tracks, most recent like first.
	want := []string{"The National", "Gorillaz", "Big Thief", "Neil Young", "Aldous Harding", "Arctic Monkeys", "Sleaford Mods"}
	if got := artistNames(artists); !equal(got, want) {
		t.Fatalf("all-time order = %v, want %v", got, want)
	}

	got := byName(artists)
	if a := got["Big Thief"]; !near(a.Score, PointsFollowed) {
		t.Errorf("Big Thief (follow only) score = %v, want %v", a.Score, PointsFollowed)
	}
	// The National's album is from 2023: all-time ignores age, so it counts in full.
	if a := got["The National"]; !near(a.Score, PointsFollowed+WeightAlbums*math.Log1p(1)) {
		t.Errorf("The National score = %v", a.Score)
	}
	if a := got["Neil Young"]; a.Signals.RecentItems != 1 || a.Recent != 0 {
		t.Errorf("Neil Young: recent items %d, recent score %v; want 1 and 0 (below RecentMinItems)",
			a.Signals.RecentItems, a.Recent)
	}

	if stats.NoArtist != 1 {
		t.Errorf("NoArtist = %d, want 1 (unavailable track in fixture)", stats.NoArtist)
	}
	if stats.NonArtistMain != 1 {
		t.Errorf("NonArtistMain = %d, want 1 (Various Artists compilation)", stats.NonArtistMain)
	}
	// "Losing Haringey" credits an artist page Tidal has removed (404): skipped, not fatal.
	if len(stats.Unresolved) != 1 || stats.Unresolved[0] != "86645327" {
		t.Errorf("Unresolved = %v, want [86645327]", stats.Unresolved)
	}
}

func TestLikesAreDamped(t *testing.T) {
	spam := credit{"1", "Heart Spammed"}
	fave := credit{"2", "Deliberate Favourite"}
	tracks := synth(t, "tracks", manyItems("t", 26, 400, spam)...)
	albums := synth(t, "albums",
		item{id: "a1", added: daysAgo(400), artists: []credit{fave}},
		item{id: "a2", added: daysAgo(400), artists: []credit{fave}})
	followed := synth(t, "artists", item{added: daysAgo(400), artists: []credit{fave}})

	artists, _ := Score(followed, albums, tracks, fixtureNow)
	if got := artistNames(artists); !equal(got, []string{"Deliberate Favourite", "Heart Spammed"}) {
		t.Errorf("order = %v: 26 hearted tracks should not beat a follow and two albums", got)
	}
}

func TestAllTimeIgnoresAge(t *testing.T) {
	old := credit{"1", "Loved In 2016"}
	fresh := credit{"2", "Loved This Week"}
	albums := synth(t, "albums",
		item{id: "a1", added: daysAgo(10 * 365), artists: []credit{old}},
		item{id: "a2", added: daysAgo(3), artists: []credit{fresh}})
	tracks := synth(t, "tracks",
		item{id: "t1", added: daysAgo(10 * 365), artists: []credit{old}},
		item{id: "t2", added: daysAgo(3), artists: []credit{fresh}})

	artists, _ := Score(synth(t, "artists"), albums, tracks, fixtureNow)
	if len(artists) != 2 || !near(artists[0].Score, artists[1].Score) {
		t.Fatalf("scores %v: the same likes should score the same whatever their age", artists)
	}
	// Recency only breaks the tie.
	if artists[0].Name != "Loved This Week" {
		t.Errorf("order = %v, want the more recent like first on a tie", artistNames(artists))
	}
	// And only the fresh one is "lately".
	if artists[0].Recent == 0 || artists[1].Recent != 0 {
		t.Errorf("recent scores %v / %v", artists[0].Recent, artists[1].Recent)
	}
}

func TestRecentArtists(t *testing.T) {
	steady := credit{"1", "Old Follow, New Albums"} // followed 2021, two albums this autumn
	binge := credit{"2", "Track Binge"}             // three tracks a month ago
	once := credit{"3", "Liked Once"}               // one track yesterday: not enough
	faded := credit{"4", "Faded"}                   // many likes, all before the window

	followed := synth(t, "artists", item{added: daysAgo(5 * 365), artists: []credit{steady}})
	albums := synth(t, "albums",
		item{id: "a1", added: daysAgo(20), artists: []credit{steady}},
		item{id: "a2", added: daysAgo(60), artists: []credit{steady}})
	tracks := synth(t, "tracks", append(append(
		manyItems("b", 3, 30, binge),
		item{id: "o", added: daysAgo(1), artists: []credit{once}}),
		manyItems("f", 10, 120, faded)...)...)

	artists, _ := Score(followed, albums, tracks, fixtureNow)
	recent := RankRecent(artists)
	if got := artistNames(recent); !equal(got, []string{"Old Follow, New Albums", "Track Binge"}) {
		t.Errorf("recent_artists = %v", got)
	}
	got := byName(artists)
	if a := got["Faded"]; a.Recent != 0 || a.Score <= got["Track Binge"].Score {
		t.Errorf("Faded: recent %v, all-time %v; want 0 recent but a higher all-time score than Track Binge",
			a.Recent, a.Score)
	}
	// The 2021 follow is outside the window: it doesn't count lately.
	rw := func(d int) float64 { return recentWeight(daysAgo(d), true, fixtureNow) }
	if a := got["Old Follow, New Albums"]; !near(a.Recent, WeightAlbums*math.Log1p(rw(20)+rw(60))) {
		t.Errorf("recent score %v, want the two albums only (old follow excluded)", a.Recent)
	}

	e := Build(artists, nil, fixtureNow)
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(e.RecentArtists) != 2 {
		t.Errorf("export recent_artists = %v", e.RecentArtists)
	}
}

func TestMergesDuplicateProfiles(t *testing.T) {
	a := credit{"100", "Sufjan Stevens"}
	b := credit{"200", "sufjan stevens "} // same artist, second Tidal profile
	tracks := synth(t, "tracks",
		item{id: "t1", added: daysAgo(10), artists: []credit{a}},
		item{id: "t2", added: daysAgo(10), artists: []credit{b}},
		item{id: "t3", added: daysAgo(10), artists: []credit{b}})
	followed := synth(t, "artists", item{added: daysAgo(10), artists: []credit{a}}, item{added: daysAgo(10), artists: []credit{b}})

	artists, stats := Score(followed, synth(t, "albums"), tracks, fixtureNow)
	if len(artists) != 1 {
		t.Fatalf("got %d artists %v, want one merged artist", len(artists), artistNames(artists))
	}
	m := artists[0]
	// 200 has three credits (follow + two tracks), 100 has two.
	if m.TidalID != "200" || len(m.AltIDs) != 1 || m.AltIDs[0] != "100" || stats.Merged != 1 {
		t.Errorf("merged = id %s alt %v (merged %d), want id 200 alt [100]", m.TidalID, m.AltIDs, stats.Merged)
	}
	if m.Signals.LikedTracks != 3 || !near(m.Score, PointsFollowed+WeightTracks*math.Log1p(3)) {
		t.Errorf("merged score %v tracks %d: want one follow and three tracks", m.Score, m.Signals.LikedTracks)
	}

	// A similar artist pointing at the alternate profile is in the library.
	other := credit{"300", "Other"}
	followed2 := synth(t, "artists", item{added: daysAgo(10), artists: []credit{other}})
	all, _ := Score(followed2, synth(t, "albums"), tracks, fixtureNow)
	e := Build(all, map[string][]tidal.Artist{"300": {{ID: "100", Name: "Sufjan Stevens (alt)"}}}, fixtureNow)
	if len(e.SimilarArtists) != 0 {
		t.Errorf("similar_artists = %v, want the merged profile excluded", e.SimilarArtists)
	}
}

func TestMainArtistAttribution(t *testing.T) {
	artists, _ := scoreFixtures(t)
	got := byName(artists)

	// Featured and secondary credits must not score.
	for _, name := range []string{
		"Johnny Marr", "Anoushka Shankar", // feat. on a Gorillaz track
		"H. Hawkline",                    // second credit on an Aldous Harding track
		"Neil Young & The Chrome Hearts", // second credit on a Neil Young album
		"War Child Records",              // label credit
		"Various Artists",                // compilation
		"Indie/Rock Playlist",            // followed playlist account
	} {
		if a, ok := got[name]; ok {
			t.Errorf("%s should not be in the export, got score %v", name, a.Score)
		}
	}

	// Aldous Harding is main on one liked track and featured on another
	// (Sleaford Mods' "Elitest G.O.A.T."): only the main credit counts.
	if a := got["Aldous Harding"]; a.Signals.LikedTracks != 1 {
		t.Errorf("Aldous Harding liked_tracks = %d, want 1", a.Signals.LikedTracks)
	}
}

func TestMainArtistID(t *testing.T) {
	tracks := loadFixture(t, "tracks.json")
	for _, it := range tracks.Items {
		r, ok := tracks.Lookup(it)
		if !ok {
			continue
		}
		if r.Attributes.Title == "The Plastic Guru (feat. Johnny Marr and Anoushka Shankar)" {
			id, ok := MainArtistID(r)
			if !ok || tracks.Included["artists:"+id].Attributes.Name != "Gorillaz" {
				t.Errorf("main artist = %q, want Gorillaz", tracks.Included["artists:"+id].Attributes.Name)
			}
			return
		}
	}
	t.Fatal("fixture track not found")
}

// ===== Build =====

func names(refs []Ref) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.Name
	}
	return out
}

func TestBuild(t *testing.T) {
	artists, _ := scoreFixtures(t)
	ids := map[string]string{}
	for _, a := range artists {
		ids[a.Name] = a.TidalID
	}

	sva := tidal.Artist{ID: "s1", Name: "Sharon Van Etten", Popularity: 0.3}
	wilco := tidal.Artist{ID: "s2", Name: "Wilco", Popularity: 0.9} // a hub: heavily discounted
	fleet := tidal.Artist{ID: "s3", Name: "Fleet Foxes"}
	beirut := tidal.Artist{ID: "s4", Name: "Beirut"}
	similar := map[string][]tidal.Artist{
		// in-library, duplicate and compilation entries must be ignored
		// "big thief" under another id is a duplicate Tidal profile: excluded by name.
		ids["The National"]: {wilco, sva, {ID: ids["Big Thief"], Name: "Big Thief"}, sva, fleet, {ID: "dup", Name: "big thief "}},
		ids["Big Thief"]:    {sva, {ID: "va", Name: "Various Artists"}, beirut},
		ids["Neil Young"]:   {wilco, sva},
	}
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.FixedZone("EST", -5*3600))
	e := Build(artists, similar, now)

	if err := e.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if e.GeneratedAt != "2026-11-21T17:00:00Z" {
		t.Errorf("generated_at = %s, want UTC", e.GeneratedAt)
	}
	if got := names(e.MyArtists); !equal(got, artistNames(artists)) {
		t.Errorf("my_artists = %v, want Score order", got)
	}

	// Sharon Van Etten: three sources. Wilco has two (National + Neil Young)
	// but its popularity drops it below Fleet Foxes (National) and Beirut (Big Thief).
	wantSim := []string{"Sharon Van Etten", "Fleet Foxes", "Beirut", "Wilco"}
	if got := names(e.SimilarArtists); !equal(got, wantSim) {
		t.Errorf("similar_artists = %v, want %v", got, wantSim)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestOutputHasNoScores(t *testing.T) {
	artists, _ := scoreFixtures(t)
	data, err := Marshal(Build(artists, nil, fixtureNow))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"generated_at", "my_artists", "recent_artists", "similar_artists"} {
		if raw[k] == nil {
			t.Errorf("missing key %s in %v", k, keys(raw))
		}
	}
	if len(raw) != 4 {
		t.Errorf("top-level keys = %v", keys(raw))
	}
	if string(raw["recent_artists"]) != "[]" {
		t.Errorf("recent_artists = %s, want [] (nothing recent enough in fixtures)", raw["recent_artists"])
	}
	var mine []map[string]any
	_ = json.Unmarshal(raw["my_artists"], &mine)
	if len(mine[0]) != 2 {
		t.Errorf("my_artists entry has fields %v, want only tidal_id and name", mine[0])
	}
}

func keys(m map[string]json.RawMessage) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestBuildIsDeterministic(t *testing.T) {
	artists, _ := scoreFixtures(t)
	similar := map[string][]tidal.Artist{
		artists[0].TidalID: {{ID: "b", Name: "B"}, {ID: "a", Name: "A"}},
		artists[1].TidalID: {{ID: "c", Name: "C"}},
	}
	first, _ := Marshal(Build(artists, similar, fixtureNow))
	for i := 0; i < 20; i++ {
		again, _ := Marshal(Build(artists, similar, fixtureNow))
		if string(again) != string(first) {
			t.Fatal("output differs between runs")
		}
	}
}

func TestValidateRejectsBadExports(t *testing.T) {
	artists, _ := scoreFixtures(t)
	good := Build(artists, nil, fixtureNow)

	cases := map[string]func(e *Export){
		"empty":           func(e *Export) { e.MyArtists = nil },
		"no name":         func(e *Export) { e.MyArtists[0].Name = "" },
		"duplicate":       func(e *Export) { e.MyArtists[1] = e.MyArtists[0] },
		"duplicate name":  func(e *Export) { e.MyArtists[1].Name = e.MyArtists[0].Name },
		"recent null":     func(e *Export) { e.RecentArtists = nil },
		"recent not mine": func(e *Export) { e.RecentArtists = []Ref{{"x", "X"}} },
		"recent dup":      func(e *Export) { e.RecentArtists = []Ref{e.MyArtists[0], e.MyArtists[0]} },
		"similar in lib":  func(e *Export) { e.SimilarArtists = []Ref{e.MyArtists[0]} },
		"similar dup":     func(e *Export) { e.SimilarArtists = []Ref{{"x", "X"}, {"x", "X"}} },
		"similar by name": func(e *Export) { e.SimilarArtists = []Ref{{"other-id", e.MyArtists[0].Name}} },
		"bad timestamp":   func(e *Export) { e.GeneratedAt = "yesterday" },
	}
	for name, mutate := range cases {
		e := good
		e.MyArtists = append([]Ref(nil), good.MyArtists...)
		mutate(&e)
		if err := e.Validate(); err == nil {
			t.Errorf("%s: Validate() = nil, want error", name)
		}
	}
}

func TestWriteAtomicAndShrinkGuard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data", "artists.json")
	artists, _ := scoreFixtures(t)
	e := Build(artists, nil, fixtureNow)

	if err := CheckShrink(path, e); err != nil {
		t.Fatalf("no previous file: %v", err)
	}
	data, _ := Marshal(e)
	if err := WriteAtomic(path, data); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(data) {
		t.Fatal("file content mismatch")
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}

	small := e
	small.MyArtists = e.MyArtists[:2]
	if err := CheckShrink(path, small); err == nil {
		t.Error("CheckShrink allowed dropping from 7 to 2 artists")
	}

	// The previous file may still be in the old format (key "artists").
	old := filepath.Join(dir, "old.json")
	_ = os.WriteFile(old, []byte(`{"artists":[{},{},{},{},{},{},{}]}`), 0o644)
	if err := CheckShrink(old, small); err == nil {
		t.Error("CheckShrink ignored an old-format previous file")
	}
	if err := CheckShrink(old, e); err != nil {
		t.Errorf("same count against old format: %v", err)
	}
}
