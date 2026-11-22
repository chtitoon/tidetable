// Command export-artists writes the scored Tidal library to a JSON file
// (default data/artists.json, which is gitignored; in CI the destination is
// set by the ARTISTS_DATA_REPO variable, see .github/workflows/daily-artists.yml).
//
// Usage: go run ./cmd/export-artists [-out data/artists.json] [-force]
//
// Reads TIDAL_* credentials from the environment (see config.Load). On any
// collection or similar-artist failure it exits non-zero and leaves the
// previous file untouched.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
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

	cols, err := fetchCollections(ctx, svc, "artists", "albums", "tracks")
	if err != nil {
		return err
	}

	now := time.Now()
	artists, stats := artistexport.Score(cols["artists"], cols["albums"], cols["tracks"], now)
	log.Printf("scored %d artists (%d items without artist, %d compilation/label credits skipped)",
		len(artists), stats.NoArtist, stats.NonArtistMain)
	if len(stats.Unresolved) > 0 {
		// Count only: workflow logs of a public repo are public, and IDs are library data.
		log.Printf("warning: skipped %d credits to artists Tidal no longer resolves",
			len(stats.Unresolved))
	}

	var wanted []string
	for _, a := range artists {
		if a.Score >= artistexport.SimilarThreshold {
			wanted = append(wanted, a.TidalID)
		}
	}
	similar, err := fetchSimilar(ctx, svc, wanted)
	if err != nil {
		return err
	}
	log.Printf("similar artists fetched for %d artists (%d rate-limited responses retried)",
		len(similar), svc.Throttled())

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

// fetchCollections reads the given collections concurrently. Each collection is
// paginated, so its own pages are still read in order.
func fetchCollections(ctx context.Context, svc *tidal.Service, kinds ...string) (map[string]tidal.Collection, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
		cols     = make(map[string]tidal.Collection, len(kinds))
	)
	for _, kind := range kinds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			col, err := svc.FetchCollection(ctx, kind)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
					cancel()
				}
				return
			}
			log.Printf("collection %s: %d items", kind, len(col.Items))
			cols[kind] = col
		}()
	}
	wg.Wait()
	return cols, firstErr
}

// fetchSimilar fetches similar artists for each id, MaxArtistBatch ids per
// request. Any artist the batch response doesn't fully cover (unavailable, or
// an incomplete similar list) is fetched on its own. If Tidal rejects a batch
// request outright (400), that batch falls back to single requests too.
func fetchSimilar(ctx context.Context, svc *tidal.Service, ids []string) (map[string][]tidal.Artist, error) {
	similar := make(map[string][]tidal.Artist, len(ids))
	batches, singles := 0, 0
	for start := 0; start < len(ids); start += tidal.MaxArtistBatch {
		batch := ids[start:min(start+tidal.MaxArtistBatch, len(ids))]
		got, err := svc.SimilarArtistsBatch(ctx, batch)
		var apiErr *tidal.APIError
		switch {
		case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusBadRequest:
			// Status only: the error body may echo the requested ids, and
			// workflow logs of a public repo are public.
			log.Printf("warning: batch similar-artists request rejected (status %d), fetching %d artists singly",
				apiErr.StatusCode, len(batch))
			got = nil
		case err != nil:
			return nil, err
		default:
			batches++
		}
		for _, id := range batch {
			if sim, ok := got[id]; ok {
				similar[id] = sim
				continue
			}
			sim, err := svc.SimilarArtists(ctx, id)
			if err != nil {
				return nil, err
			}
			similar[id] = sim
			singles++
		}
	}
	log.Printf("similar artists: %d batch requests, %d single-artist fallbacks", batches, singles)
	return similar, nil
}
