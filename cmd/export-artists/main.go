// Command export-artists writes the scored Tidal library to data/artists.json.
//
// Usage: go run ./cmd/export-artists [-out data/artists.json] [-force]
//
// Reads TIDAL_* credentials from the environment (see config.Load). On any
// collection or similar-artist failure it exits non-zero and leaves the
// previous file untouched.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/chtitoon/tidetable/artistexport"
	"github.com/chtitoon/tidetable/config"
	"github.com/chtitoon/tidetable/tidal"
)

func main() {
	out := flag.String("out", "data/artists.json", "output file")
	force := flag.Bool("force", false, "write even if the artist count dropped by more than half")
	flag.Parse()

	if err := run(*out, *force); err != nil {
		log.Fatalf("export failed: %v", err)
	}
}

func run(out string, force bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	auth := tidal.NewAuthenticator(cfg.TidalClientID, cfg.TidalClientSecret, "")
	auth.SetTokens(cfg.TidalAccessToken, cfg.TidalRefreshToken)
	svc := tidal.NewService(auth)

	cols := map[string]tidal.Collection{}
	for _, kind := range []string{"artists", "albums", "tracks"} {
		col, err := svc.FetchCollection(ctx, kind)
		if err != nil {
			return err
		}
		log.Printf("collection %s: %d items", kind, len(col.Items))
		cols[kind] = col
	}

	now := time.Now()
	artists, stats := artistexport.Score(cols["artists"], cols["albums"], cols["tracks"], now)
	log.Printf("scored %d artists (%d items without artist, %d compilation/label credits skipped)",
		len(artists), stats.NoArtist, stats.NonArtistMain)
	if len(stats.Unresolved) > 0 {
		log.Printf("warning: skipped %d credits to artists Tidal no longer resolves: %v",
			len(stats.Unresolved), stats.Unresolved)
	}

	threshold := artistexport.SimilarThreshold
	similar := map[string][]tidal.Artist{}
	n := 0
	for _, a := range artists {
		if a.Score < threshold {
			continue
		}
		sim, err := svc.SimilarArtists(ctx, a.TidalID)
		if err != nil {
			return err
		}
		similar[a.TidalID] = sim
		if n++; n%50 == 0 {
			log.Printf("similar artists: %d done", n)
		}
	}
	log.Printf("similar artists fetched for %d artists", n)

	export := artistexport.Build(artists, similar, now)
	if err := export.Validate(); err != nil {
		return err
	}
	if !force {
		if err := artistexport.CheckShrink(out, export); err != nil {
			return err
		}
	}
	data, err := artistexport.Marshal(export)
	if err != nil {
		return err
	}
	if err := artistexport.WriteAtomic(out, data); err != nil {
		return err
	}
	log.Printf("wrote %s: %d my_artists, %d similar_artists", out, len(export.MyArtists), len(export.SimilarArtists))
	return nil
}
