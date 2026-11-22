// Command tidetable scrapes the venues in venues.yaml and writes every show
// in the next ~4 months to data/shows.json. It needs no credentials.
//
//	go run .                      # writes data/shows.json
//	go run . -out /tmp/shows.json
//	go run . -months 6
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
	months := flag.Int("months", 4, "how many months ahead to include")
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
	start := now.In(ny)
	end := start.AddDate(0, *months, 0)
	fmt.Printf("Scraping %d venue(s) for shows from %s to %s...\n",
		len(venueNames), start.Format("Mon Jan 2, 2006"), end.Format("Mon Jan 2, 2006"))

	results, err := lescraper.ScrapeVenues(venueNames, start, end)
	if err != nil {
		log.Fatalf("Failed to scrape venues: %v", err)
	}

	doc := showexport.Build(results, now)
	for _, r := range results {
		if r.Err != nil {
			fmt.Printf("  ✗ %-28s %v\n", r.Name, r.Err)
			continue
		}
		note := ""
		if r.Dropped > 0 {
			note = fmt.Sprintf(" (%d non-concerts dropped)", r.Dropped)
		}
		fmt.Printf("  ✓ %-28s %4d shows%s\n", r.Name, len(r.Shows), note)
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
