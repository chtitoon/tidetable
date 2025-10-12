package venues

import (
	"regexp"
	"strings"
	"time"
)

// NormalizedShow represents a show with cleaned and extracted artist information
type NormalizedShow struct {
	Artists  []string // Multiple artists extracted from the show title
	Venue    string
	Date     time.Time
	Price    string
	URL      string
	VenueURL string
}

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

// extractArtists intelligently extracts individual artist names from a show title
func extractArtists(title string) []string {
	// Clean up the title first
	cleaned := cleanTitle(title)

	// Split by common separators
	artists := splitArtists(cleaned)

	// Clean each artist name
	var result []string
	for _, artist := range artists {
		artist = strings.TrimSpace(artist)
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

// cleanTitle removes common suffixes and annotations from show titles
func cleanTitle(title string) string {
	title = strings.TrimSpace(title)

	// Remove common performance annotations (case-insensitive)
	// Examples: "(Live)", "- Live", "- Live 2025", "(DJ Set)", "- Acoustic", etc.
	patterns := []string{
		`(?i)\s*\(live\)\s*$`,
		`(?i)\s*-\s*live\s*\d*\s*$`,
		`(?i)\s*\(dj set\)\s*$`,
		`(?i)\s*-\s*dj set\s*\d*\s*$`,
		`(?i)\s*\(acoustic\)\s*$`,
		`(?i)\s*-\s*acoustic\s*\d*\s*$`,
		`(?i)\s*\(solo\)\s*$`,
		`(?i)\s*-\s*solo\s*\d*\s*$`,
		`(?i)\s*\(full band\)\s*$`,
		`(?i)\s*-\s*full band\s*\d*\s*$`,
		`(?i)\s*\(unplugged\)\s*$`,
		`(?i)\s*-\s*unplugged\s*\d*\s*$`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		title = re.ReplaceAllString(title, "")
	}

	// Remove tour names in format "Artist Name - Tour Name Tour"
	// Example: "Artist Name - World Tour 2025" -> "Artist Name"
	tourPattern := regexp.MustCompile(`(?i)\s*[-–]\s*.*?(tour|concert|show|presents)\s*\d*\s*$`)
	title = tourPattern.ReplaceAllString(title, "")

	return strings.TrimSpace(title)
}

// splitArtists splits a title into multiple artists based on common separators
func splitArtists(title string) []string {
	var artists []string

	// Check for "w/" or "with" separator (main act w/ support)
	// Example: "Headliner w/ Support Act" or "Artist with Special Guest"
	withPattern := regexp.MustCompile(`(?i)\s+(?:w/|with)\s+`)
	if withPattern.MatchString(title) {
		parts := withPattern.Split(title, -1)
		for _, part := range parts {
			artists = append(artists, strings.TrimSpace(part))
		}
		return artists
	}

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
	andPattern := regexp.MustCompile(`\s+(?:&|and)\s+`)
	if andPattern.MatchString(title) {
		parts := andPattern.Split(title, -1)
		// Only split if we get reasonable number of artists (2-4)
		if len(parts) >= 2 && len(parts) <= 4 {
			for _, part := range parts {
				artists = append(artists, strings.TrimSpace(part))
			}
			return artists
		}
	}

	// No separator found, return the whole title as single artist
	return []string{title}
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
	ignored := []string{
		"and more",
		"tba",
		"to be announced",
		"special guests",
		"guests",
		"more",
		"",
	}

	for _, phrase := range ignored {
		if s == phrase {
			return true
		}
	}

	return false
}
