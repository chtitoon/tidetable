package artistexport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chtitoon/tidetable/tidal"
)

// fixtureNow is "today" for the fixtures: the 6-month window starts 2026-05-23.
var fixtureNow = time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)

// loadFixture reads a recorded JSON:API page (trimmed real responses from
// /v2/userCollection*/me/relationships/items, with meta.addedAt) into a Collection.
func loadFixture(t *testing.T, name string) tidal.Collection {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
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

func TestScore(t *testing.T) {
	artists, stats := scoreFixtures(t)

	want := []struct {
		name   string
		score  int
		recent int
	}{
		{"The National", 5, 0},   // follow 3 (2024) + album 2 (2023)
		{"Neil Young", 4, 1},     // album liked 2026-11-07: 2 x2
		{"Gorillaz", 4, 0},       // follow 3 (2021) + track 1 (2026-04-20, just outside 6 months)
		{"Big Thief", 3, 0},      // follow 3 (2021)
		{"Aldous Harding", 2, 1}, // track liked 2026-06-09: 1 x2
		{"Arctic Monkeys", 1, 0}, // track 2026-03-24
		{"Sleaford Mods", 1, 0},  // track 2026-03-08 (older than Arctic Monkeys' like)
	}
	if len(artists) != len(want) {
		names := []string{}
		for _, a := range artists {
			names = append(names, a.Name)
		}
		t.Fatalf("got %d artists %v, want %d", len(artists), names, len(want))
	}
	// Order matters: score desc, then most recent like, then name.
	for i, w := range want {
		a := artists[i]
		if a.Name != w.name || a.Score != w.score || a.Signals.RecentItems != w.recent {
			t.Errorf("#%d = %s score %d recent %d, want %s score %d recent %d",
				i, a.Name, a.Score, a.Signals.RecentItems, w.name, w.score, w.recent)
		}
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

func TestRecencyDoublesPoints(t *testing.T) {
	// The same library scored a year later: nothing is recent any more.
	later := fixtureNow.AddDate(1, 0, 0)
	artists, _ := Score(loadFixture(t, "followed.json"), loadFixture(t, "albums.json"), loadFixture(t, "tracks.json"), later)
	got := byName(artists)
	if a := got["Neil Young"]; a.Score != 2 || a.Signals.RecentItems != 0 {
		t.Errorf("Neil Young a year later: score %d recent %d, want 2 and 0", a.Score, a.Signals.RecentItems)
	}
	if a := got["Aldous Harding"]; a.Score != 1 {
		t.Errorf("Aldous Harding a year later: score %d, want 1", a.Score)
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
			t.Errorf("%s should not be in the export, got score %d", name, a.Score)
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

	sva := tidal.Artist{ID: "s1", Name: "Sharon Van Etten"}
	wilco := tidal.Artist{ID: "s2", Name: "Wilco"}
	fleet := tidal.Artist{ID: "s3", Name: "Fleet Foxes"}
	beirut := tidal.Artist{ID: "s4", Name: "Beirut"}
	similar := map[string][]tidal.Artist{
		// in-library, duplicate and compilation entries must be ignored
		ids["The National"]: {wilco, sva, {ID: ids["Big Thief"], Name: "Big Thief"}, sva, fleet},
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

	wantMine := []string{"The National", "Neil Young", "Gorillaz", "Big Thief", "Aldous Harding", "Arctic Monkeys", "Sleaford Mods"}
	if got := names(e.MyArtists); !equal(got, wantMine) {
		t.Errorf("my_artists = %v, want %v", got, wantMine)
	}

	// Sharon Van Etten: 3 cross-refs. Wilco: 2 (National 5 + Neil Young 4).
	// Fleet Foxes (National, 5) beats Beirut (Big Thief, 3) on source score.
	wantSim := []string{"Sharon Van Etten", "Wilco", "Fleet Foxes", "Beirut"}
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
	if len(raw) != 3 || raw["generated_at"] == nil || raw["my_artists"] == nil || raw["similar_artists"] == nil {
		t.Errorf("top-level keys = %v, want generated_at, my_artists, similar_artists", keys(raw))
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
		"empty":          func(e *Export) { e.MyArtists = nil },
		"no name":        func(e *Export) { e.MyArtists[0].Name = "" },
		"duplicate":      func(e *Export) { e.MyArtists[1] = e.MyArtists[0] },
		"similar in lib": func(e *Export) { e.SimilarArtists = []Ref{e.MyArtists[0]} },
		"similar dup":    func(e *Export) { e.SimilarArtists = []Ref{{"x", "X"}, {"x", "X"}} },
		"bad timestamp":  func(e *Export) { e.GeneratedAt = "yesterday" },
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
