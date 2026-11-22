// Package showexport builds and writes data/shows.json from scrape results.
package showexport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/chtitoon/tidetable/lescraper"
	"github.com/chtitoon/tidetable/lescraper/core"
)

// Doc is the shows.json document.
type Doc struct {
	GeneratedAt string  `json:"generated_at"`
	Venues      []Venue `json:"venues"`
	Shows       []Show  `json:"shows"`
}

// Venue reports how scraping one configured venue went.
type Venue struct {
	Name  string  `json:"name"`
	URL   string  `json:"url"`
	OK    bool    `json:"ok"`
	Shows int     `json:"shows"`
	Error *string `json:"error"`
}

// Show is one listing.
type Show struct {
	Date      string   `json:"date"` // YYYY-MM-DD
	Time      *string  `json:"time"` // HH:MM, or null
	Venue     string   `json:"venue"`
	Room      *string  `json:"room"`
	Headliner string   `json:"headliner"`
	Support   []string `json:"support"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	OnSaleAt  *string  `json:"on_sale_at"`
	URL       string   `json:"url"`
}

// Build assembles the document. Venues keep their configured order; shows
// are sorted by date, venue, time, room, title, URL so diffs stay readable.
func Build(results []lescraper.VenueResult, generatedAt time.Time) Doc {
	doc := Doc{
		GeneratedAt: generatedAt.UTC().Format(time.RFC3339),
		Venues:      []Venue{},
		Shows:       []Show{},
	}
	for _, r := range results {
		v := Venue{Name: r.Name, URL: r.URL, OK: r.Err == nil}
		if r.Err != nil {
			msg := r.Err.Error()
			v.Error = &msg
		} else {
			v.Shows = len(r.Shows)
			for _, s := range r.Shows {
				doc.Shows = append(doc.Shows, convert(s))
			}
		}
		doc.Venues = append(doc.Venues, v)
	}
	sort.SliceStable(doc.Shows, func(i, j int) bool {
		a, b := doc.Shows[i], doc.Shows[j]
		for _, c := range [][2]string{
			{a.Date, b.Date}, {a.Venue, b.Venue}, {deref(a.Time), deref(b.Time)},
			{deref(a.Room), deref(b.Room)}, {a.Title, b.Title}, {a.URL, b.URL},
		} {
			if c[0] != c[1] {
				return c[0] < c[1]
			}
		}
		return false
	})
	return doc
}

func convert(s core.Show) Show {
	headliner, support := core.Lineup(s)
	status := s.Status
	if status == "" {
		status = core.StatusUnknown
	}
	out := Show{
		Date:      s.Date.Format("2006-01-02"),
		Time:      ptr(s.Time),
		Venue:     s.Venue,
		Room:      ptr(s.Room),
		Headliner: headliner,
		Support:   support,
		Title:     s.Title,
		Status:    string(status),
		URL:       s.URL,
	}
	if !s.OnSaleAt.IsZero() {
		out.OnSaleAt = ptr(s.OnSaleAt.UTC().Format(time.RFC3339))
	}
	return out
}

// AllFailed reports whether every venue failed.
func (d Doc) AllFailed() bool {
	for _, v := range d.Venues {
		if v.OK {
			return false
		}
	}
	return true
}

// Write writes the document to path atomically (temp file + rename).
func Write(path string, doc Doc) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".shows-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
