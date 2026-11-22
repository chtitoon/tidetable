package artistexport

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Explain writes a score breakdown of the top n artists of both rankings, for
// tuning the scoring locally. It prints artist names and library details:
// never call it in CI, whose logs may be public.
func Explain(w io.Writer, artists []Artist, n int) error {
	section := func(title string, list []Artist, score func(Artist) (float64, Parts)) error {
		fmt.Fprintf(w, "\n%s\n", title)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
		fmt.Fprintln(tw, "#\tscore\t= follow\t+ albums\t+ tracks\t\tfollowed\talbums\ttracks\tlast 90d\tlatest like\t")
		for i, a := range list[:min(n, len(list))] {
			s, p := score(a)
			followed := ""
			if a.Signals.Followed {
				followed = "yes"
			}
			latest := ""
			if !a.Signals.LatestAdded.IsZero() {
				latest = a.Signals.LatestAdded.Format("2006-01-02")
			}
			name := a.Name
			if len(a.AltIDs) > 0 {
				name += fmt.Sprintf(" (+%d merged: %s)", len(a.AltIDs), strings.Join(a.AltIDs, ", "))
			}
			// The name goes after the last cell so it stays left-aligned.
			fmt.Fprintf(tw, "%d\t%.2f\t%.2f\t%.2f\t%.2f\t\t%s\t%d\t%d\t%d\t%s\t  %s\n",
				i+1, s, p.Follow, p.Albums, p.Tracks,
				followed, a.Signals.LikedAlbums, a.Signals.LikedTracks, a.Signals.RecentItems, latest, name)
		}
		return tw.Flush()
	}
	if err := section("my_artists (all-time)", artists,
		func(a Artist) (float64, Parts) { return a.Score, a.AllTimeParts }); err != nil {
		return err
	}
	return section("recent_artists (last 90 days)", RankRecent(artists),
		func(a Artist) (float64, Parts) { return a.Recent, a.RecentParts })
}
