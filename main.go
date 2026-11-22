// Command tidetable scrapes the venues in venues.yaml and writes every
// upcoming show they list, however far out, to data/shows.json. It needs no
// credentials.
//
//	go run .                      # writes data/shows.json
//	go run . -out /tmp/shows.json
//
// A venue that fails is recorded with ok:false and its error; the run exits
// non-zero only if every venue fails.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	_ "time/tzdata" // America/New_York even where the OS has no zoneinfo

	"github.com/chtitoon/tidetable/config"
	"github.com/chtitoon/tidetable/lescraper"
	"github.com/chtitoon/tidetable/showexport"
)

func main() {
	out := flag.String("out", "data/shows.json", "output file")
	flag.Parse()

	venuesFile := os.Getenv("VENUES_FILE")
	if venuesFile == "" {
		venuesFile = "venues.yaml"
	}
	venueNames, err := config.LoadVenues(venuesFile)
	if err != nil {
		log.Fatalf("Failed to load venues: %v", err)
	}

	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		log.Fatalf("Loading time zone: %v", err)
	}
	now := time.Now()
	today := now.In(ny)
	fmt.Printf("Scraping %d venue(s) for every show from %s on...\n",
		len(venueNames), today.Format("Mon Jan 2, 2006"))

	results, err := lescraper.ScrapeVenues(venueNames, today)
	if err != nil {
		log.Fatalf("Failed to scrape venues: %v", err)
	}

	doc := showexport.Build(results, now)
	for i, r := range results {
		if r.Err != nil && len(r.Shows) == 0 {
			fmt.Printf("  ✗ %-28s %v\n", r.Name, r.Err)
			continue
		}
		var notes []string
		if r.Dropped > 0 {
			notes = append(notes, fmt.Sprintf("%d non-concerts dropped", r.Dropped))
		}
		if r.Skipped > 0 {
			notes = append(notes, fmt.Sprintf("%d placeholders skipped", r.Skipped))
		}
		mark, note := "✓", ""
		if len(notes) > 0 {
			note = " (" + strings.Join(notes, ", ") + ")"
		}
		if r.Err != nil { // incomplete listing: shows kept, venue flagged
			mark, note = "✗", note+": "+r.Err.Error()
		}
		fmt.Printf("  %s %-28s %4d shows through %s%s\n", mark, r.Name, len(r.Shows), *doc.Venues[i].LatestDate, note)
	}

	if doc.AllFailed() {
		// Keep the previous file: an all-failed export would wipe it.
		log.Fatalf("Every venue failed; not writing %s", *out)
	}
	if err := showexport.Write(*out, doc); err != nil {
		log.Fatalf("Writing %s: %v", *out, err)
	}

	failed := []string{}
	for _, v := range doc.Venues {
		if !v.OK {
			failed = append(failed, v.Name)
		}
	}
	fmt.Printf("\nWrote %d shows to %s", len(doc.Shows), *out)
	if len(failed) > 0 {
		fmt.Printf(" (%d venue(s) failed: %s)", len(failed), strings.Join(failed, ", "))
	}
	fmt.Println()
}
