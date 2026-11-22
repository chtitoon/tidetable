package artistexport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chtitoon/tidetable/tidal"
)

// loadFixture reads a recorded JSON:API page (trimmed real responses from
// /v2/userCollection*/me/relationships/items) into a Collection.
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
	return Score(loadFixture(t, "followed.json"), loadFixture(t, "albums.json"), loadFixture(t, "tracks.json"))
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
		name    string
		score   int
		signals Signals
	}{
		{"The National", 5, Signals{Followed: true, LikedAlbums: 1}},
		{"Gorillaz", 4, Signals{Followed: true, LikedTracks: 1}},
		{"Big Thief", 3, Signals{Followed: true}},
		{"Neil Young", 2, Signals{LikedAlbums: 1}},
		{"Aldous Harding", 1, Signals{LikedTracks: 1}},
		{"Arctic Monkeys", 1, Signals{LikedTracks: 1}},
		{"Sleaford Mods", 1, Signals{LikedTracks: 1}},
	}
	if len(artists) != len(want) {
		names := []string{}
		for _, a := range artists {
			names = append(names, a.Name)
		}
		t.Fatalf("got %d artists %v, want %d", len(artists), names, len(want))
	}
	// Order matters: score desc, then name.
	for i, w := range want {
		a := artists[i]
		if a.Name != w.name || a.Score != w.score || a.Signals != w.signals {
			t.Errorf("#%d = %s score %d %+v, want %s score %d %+v", i, a.Name, a.Score, a.Signals, w.name, w.score, w.signals)
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

func TestBuildSimilarAndCandidates(t *testing.T) {
	artists, _ := scoreFixtures(t)
	ids := map[string]string{}
	for _, a := range artists {
		ids[a.Name] = a.TidalID
	}

	sva := tidal.Artist{ID: "s1", Name: "Sharon Van Etten"}
	wilco := tidal.Artist{ID: "s2", Name: "Wilco"}
	similar := map[string][]tidal.Artist{
		ids["The National"]: {wilco, sva, {ID: ids["Big Thief"], Name: "Big Thief"}, sva /* duplicate */},
		ids["Big Thief"]:    {sva, {ID: "s3", Name: "Various Artists"}},
		ids["Neil Young"]:   {wilco, sva},
	}
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.FixedZone("EST", -5*3600))
	e := Build(artists, similar, DefaultThreshold, now)

	if err := e.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if e.GeneratedAt != "2026-11-21T17:00:00Z" {
		t.Errorf("generated_at = %s, want UTC", e.GeneratedAt)
	}

	nat := byName(e.Artists)["The National"]
	wantSim := []SimilarRef{
		{ids["Big Thief"], "Big Thief", true},
		{"s1", "Sharon Van Etten", false},
		{"s2", "Wilco", false},
	}
	if len(nat.Similar) != len(wantSim) {
		t.Fatalf("The National similar = %+v, want %+v", nat.Similar, wantSim)
	}
	for i := range wantSim {
		if nat.Similar[i] != wantSim[i] {
			t.Errorf("similar[%d] = %+v, want %+v", i, nat.Similar[i], wantSim[i])
		}
	}
	if sim := byName(e.Artists)["Aldous Harding"].Similar; sim == nil || len(sim) != 0 {
		t.Errorf("below-threshold artist similar = %#v, want empty non-nil list", sim)
	}

	// Sharon Van Etten: 3 sources (5+3+2=10). Wilco: 2 sources (5+2=7).
	want := []SimilarCandidate{
		{"s1", "Sharon Van Etten", 3, 10, []string{"Big Thief", "Neil Young", "The National"}},
		{"s2", "Wilco", 2, 7, []string{"Neil Young", "The National"}},
	}
	if len(e.SimilarCandidates) != len(want) {
		t.Fatalf("candidates = %+v, want %+v", e.SimilarCandidates, want)
	}
	for i, w := range want {
		g := e.SimilarCandidates[i]
		if g.TidalID != w.TidalID || g.Weight != w.Weight || g.SourceScore != w.SourceScore || len(g.Sources) != len(w.Sources) {
			t.Errorf("candidate[%d] = %+v, want %+v", i, g, w)
			continue
		}
		for j := range w.Sources {
			if g.Sources[j] != w.Sources[j] {
				t.Errorf("candidate[%d].sources = %v, want %v", i, g.Sources, w.Sources)
			}
		}
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	artists, _ := scoreFixtures(t)
	similar := map[string][]tidal.Artist{artists[0].TidalID: {{ID: "b", Name: "B"}, {ID: "a", Name: "A"}}}
	now := time.Unix(0, 0)
	first, _ := Marshal(Build(artists, similar, 2, now))
	for i := 0; i < 20; i++ {
		again, _ := Marshal(Build(artists, similar, 2, now))
		if string(again) != string(first) {
			t.Fatal("output differs between runs")
		}
	}
}

func TestValidateRejectsBadExports(t *testing.T) {
	artists, _ := scoreFixtures(t)
	good := Build(artists, nil, DefaultThreshold, time.Now())

	cases := map[string]func(e *Export){
		"empty":          func(e *Export) { e.Artists = nil },
		"wrong score":    func(e *Export) { e.Artists[0].Score++ },
		"duplicate":      func(e *Export) { e.Artists[1] = e.Artists[0] },
		"bad in_library": func(e *Export) { e.Artists[0].Similar = []SimilarRef{{"zzz", "Nobody", true}} },
		"candidate in lib": func(e *Export) {
			e.SimilarCandidates = []SimilarCandidate{{e.Artists[0].TidalID, e.Artists[0].Name, 1, 1, []string{"x"}}}
		},
	}
	for name, mutate := range cases {
		e := good
		e.Artists = append([]Artist(nil), good.Artists...)
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
	e := Build(artists, nil, DefaultThreshold, time.Now())

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
	small.Artists = e.Artists[:2]
	if err := CheckShrink(path, small); err == nil {
		t.Error("CheckShrink allowed dropping from 7 to 2 artists")
	}
}
