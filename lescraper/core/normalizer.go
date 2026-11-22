package core

import (
	"regexp"
	"strings"
)

// NormalizeShows processes raw shows and extracts individual artists
// Handles cases like:
// - "Artist A, Artist B, Artist C" -> ["Artist A", "Artist B", "Artist C"]
// - "Artist Name (Live)" -> ["Artist Name"]
// - "Artist - Live at Venue" -> ["Artist"]
// - "Artist w/ Support Act" -> ["Artist", "Support Act"]
func NormalizeShows(shows []Show) []NormalizedShow {
	var normalized []NormalizedShow

	for _, show := range shows {
		artists := extractArtists(show.Title)

		// Create a normalized show entry
		normalized = append(normalized, NormalizedShow{
			Artists:  artists,
			Venue:    show.Venue,
			Date:     show.Date,
			Price:    show.Price,
			URL:      show.URL,
			VenueURL: show.VenueURL,
		})
	}

	return normalized
}

// SplitLineup splits a show title into a headliner and support acts,
// best-effort: "Friendship & Charlotte Cornfield" -> "Friendship", ["Charlotte Cornfield"].
func SplitLineup(title string) (string, []string) {
	artists := extractArtists(title)
	return artists[0], artists[1:]
}

// Lineup returns the show's headliner and support acts. An explicit
// Headliner from the source wins; otherwise the title is split with
// SplitLineup. Explicit Support entries are merged in, without duplicates.
func Lineup(s Show) (string, []string) {
	headliner := strings.TrimSpace(s.Headliner)
	var support []string
	if headliner == "" {
		headliner, support = SplitLineup(s.Title)
	}
	seen := map[string]bool{strings.ToLower(headliner): true}
	out := []string{}
	for _, a := range append(support, s.Support...) {
		a = cleanArtist(a)
		k := strings.ToLower(a)
		if a == "" || seen[k] || isIgnoredPhrase(a) {
			continue
		}
		seen[k] = true
		out = append(out, a)
	}
	return headliner, out
}

// extractArtists intelligently extracts individual artist names from a show title
func extractArtists(title string) []string {
	// Clean up the title first
	cleaned := cleanTitle(title)

	// Split by common separators
	artists := splitArtists(cleaned)

	// Clean each artist name
	var result []string
	for _, artist := range artists {
		artist = cleanArtist(artist)
		if artist != "" && !isIgnoredPhrase(artist) {
			result = append(result, artist)
		}
	}

	// If no artists were extracted, return the original cleaned title
	if len(result) == 0 {
		return []string{cleaned}
	}

	return result
}

var (
	// Performance annotations (case-insensitive), e.g. "(Live)", "- Acoustic 2025".
	annotationPatterns = func() []*regexp.Regexp {
		var res []*regexp.Regexp
		for _, word := range []string{"live", "dj set", "acoustic", "solo", "full band", "unplugged"} {
			res = append(res,
				regexp.MustCompile(`(?i)\s*\(`+word+`\)\s*$`),
				regexp.MustCompile(`(?i)\s*-\s*`+word+`\s*\d*\s*$`))
		}
		return res
	}()

	// Tour names in various formats:
	// "Artist Name - Tour Name Tour" -> "Artist Name"
	// "Artist Name: Tour Name Tour" -> "Artist Name"
	// "LOJAY: XOXO Tour" -> "LOJAY"
	tourPattern = regexp.MustCompile(`(?i)\s*[-–:]\s*.*?(tour|concert|show|presents)\s*\d*\s*$`)

	// "An Evening with Richard Marx" -> "Richard Marx"
	eveningPattern = regexp.MustCompile(`(?i)^an evening with\s+`)

	// The first subtitle separator: "Artist: Subtitle", "Artist - Subtitle",
	// "Artist – Subtitle", "Artist / Subtitle". Spaced dashes only, so
	// hyphenated names survive.
	subtitlePattern = regexp.MustCompile(`^(.+?)(?::\s+|\s+[-–—/]\s+)(.+)$`)

	// Main act w/ support: "Headliner w/ Support Act", "Artist with Special Guest",
	// "Show ft. Artist"
	withPattern = regexp.MustCompile(`(?i)\s+(?:w/|with|ft\.|feat\.|featuring)\s+`)

	// Only "&" separates acts; a lowercase " and " is too often part of a
	// title ("Wild and Swingin' Holiday Party").
	andPattern = regexp.MustCompile(`\s+&\s+`)

	// Prefixes on individual names: "and Besche", "With Special Guest Macy Todd"
	artistPrefixPattern = regexp.MustCompile(`(?i)^(?:(?:and|&|with)\s+)?(?:(?:very\s+)?special\s+guests?:?\s+)?`)

	// "Low Cut Connie (solo) 107.1 The Peak's Holiday Hang" -> "Low Cut Connie"
	artistAnnotationTail = regexp.MustCompile(`(?i)\s*\((?:solo|live|acoustic|dj set|full band|unplugged)\).*$`)

	// A list of acts: "A, B, C" or "A + B"
	listPattern = regexp.MustCompile(`,|\s\+\s`)
)

// cleanTitle removes common suffixes and annotations from show titles
func cleanTitle(title string) string {
	title = strings.TrimSpace(title)

	for _, re := range annotationPatterns {
		title = re.ReplaceAllString(title, "")
	}
	title = tourPattern.ReplaceAllString(title, "")
	title = eveningPattern.ReplaceAllString(title, "")

	// "Darlene Love: Love For The Holidays" -> "Darlene Love", but a series
	// name before a list keeps the list: "The Well: Paul Babe, John Dorney".
	if m := subtitlePattern.FindStringSubmatch(title); m != nil {
		head, rest := strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
		if listPattern.MatchString(rest) && !listPattern.MatchString(head) {
			title = rest
		} else {
			title = head
		}
	}

	return strings.TrimSpace(title)
}

// cleanArtist trims one extracted name.
func cleanArtist(s string) string {
	s = strings.TrimSpace(s)
	s = artistPrefixPattern.ReplaceAllString(s, "")
	s = artistAnnotationTail.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// splitArtists splits a title into multiple artists based on common separators
func splitArtists(title string) []string {
	var artists []string

	if withPattern.MatchString(title) {
		parts := withPattern.Split(title, -1)
		for _, part := range parts {
			// Each side may itself be a list: "A, B w/ C, D"
			artists = append(artists, splitList(part)...)
		}
		return artists
	}

	return splitList(title)
}

// splitList splits on commas, " + ", then " & " / " and ".
func splitList(title string) []string {
	var artists []string

	// Check for comma-separated artists
	// Example: "Artist A, Artist B, Artist C"
	if strings.Contains(title, ",") {
		parts := strings.Split(title, ",")
		for _, part := range parts {
			// Skip if this looks like a tour name or subtitle
			part = strings.TrimSpace(part)
			if !looksLikeTourName(part) {
				artists = append(artists, part)
			}
		}
		if len(artists) > 0 {
			return artists
		}
	}

	// Check for " + " separator
	// Example: "Artist A + Artist B"
	if strings.Contains(title, " + ") {
		parts := strings.Split(title, " + ")
		for _, part := range parts {
			artists = append(artists, strings.TrimSpace(part))
		}
		return artists
	}

	// Check for " & " or " and " separator
	// Example: "Artist A & Artist B" or "Artist A and Artist B"
	if andPattern.MatchString(title) {
		parts := andPattern.Split(title, -1)
		// Only split if we get reasonable number of artists (2-4)
		if len(parts) >= 2 && len(parts) <= 4 && splittableOnAnd(parts) {
			for _, part := range parts {
				artists = append(artists, strings.TrimSpace(part))
			}
			return artists
		}
	}

	// No separator found, return the whole title as single artist
	return []string{title}
}

// splittableOnAnd guards against splitting band names that contain "&":
// "Annalise & The Mother Underground", "Emmett & the Dead Ends" (X & the Ys),
// and "Milk & Bone" (single word on both sides).
func splittableOnAnd(parts []string) bool {
	allSingleWords := true
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if i > 0 && strings.HasPrefix(strings.ToLower(p), "the ") {
			return false
		}
		if strings.Contains(p, " ") {
			allSingleWords = false
		}
	}
	return !allSingleWords
}

// looksLikeTourName checks if a string looks like a tour name rather than an artist
func looksLikeTourName(s string) bool {
	s = strings.ToLower(s)
	tourKeywords := []string{
		"tour",
		"concert",
		"show",
		"presents",
		"performing",
		"live at",
		"album release",
		"record release",
		"special guest",
	}

	for _, keyword := range tourKeywords {
		if strings.Contains(s, keyword) {
			return true
		}
	}

	return false
}

// isIgnoredPhrase checks if a string is a common phrase that should be ignored
func isIgnoredPhrase(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimRight(s, "!.… ")
	ignored := []string{
		"and more",
		"more",
		"tba",
		"to be announced",
		"special guests",
		"special guest",
		"guests",
		"friends",
		"",
	}

	for _, phrase := range ignored {
		if s == phrase {
			return true
		}
	}

	return false
}
