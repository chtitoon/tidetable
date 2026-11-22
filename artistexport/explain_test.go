package artistexport

import (
	"strings"
	"testing"
)

func TestExplain(t *testing.T) {
	artists, _ := scoreFixtures(t)
	for _, a := range artists {
		if !near(a.AllTimeParts.Total(), a.Score) || !near(a.RecentParts.Total(), a.Recent) {
			t.Errorf("%s: parts don't add up to the scores", a.Name)
		}
	}

	var b strings.Builder
	if err := Explain(&b, artists, 3); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	t.Log("\n" + out)
	for _, want := range []string{"my_artists", "recent_artists", "The National", "Big Thief"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if strings.Contains(out, "Neil Young") {
		t.Error("printed more than the top 3")
	}
}
