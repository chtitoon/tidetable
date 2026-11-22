package core

import (
	"errors"
	"strings"
	"unicode"
)

// ErrIncomplete marks a scrape that stopped before the end of the venue's
// listing (page cap hit, feed shorter than its stated total). A scraper
// returns it wrapped, together with the shows it did get: those are kept,
// and the venue is reported as failed with this error.
var ErrIncomplete = errors.New("listing incomplete")

// placeholderNames are lineup entries that name no act, compared after
// placeholderKey normalization.
var placeholderNames = map[string]bool{
	"tba": true, "tbd": true, "tbc": true,
	"to be announced": true, "to be determined": true, "to be confirmed": true,
	"special guest": true, "special guests": true,
	"very special guest": true, "very special guests": true,
	"special guest tba": true, "special guests tba": true,
	"guest": true, "guests": true, "more tba": true, "artist tba": true,
}

// IsPlaceholderName reports whether a headliner or support entry names no
// real act: fewer than 3 letters or digits ("@", "?", "X"), or a placeholder
// such as "TBA", "To be announced" or "Special Guests".
func IsPlaceholderName(name string) bool {
	alnum := 0
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			alnum++
		}
	}
	key := placeholderKey(name)
	compact := strings.ReplaceAll(key, " ", "") // "T.B.A." -> "tba"
	return alnum < 3 || placeholderNames[key] || compact == "tba" || compact == "tbd" || compact == "tbc"
}

// placeholderKey lowercases s, turns punctuation into spaces and collapses
// whitespace, then rejoins a split-off plural "s":
// "Special Guest(s)!" -> "special guests".
func placeholderKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	words := strings.Fields(b.String())
	// "guest(s)" -> "guest s" -> "guests"
	for i := len(words) - 1; i > 0; i-- {
		if words[i] == "s" {
			words[i-1] += "s"
			words = append(words[:i], words[i+1:]...)
		}
	}
	return strings.Join(words, " ")
}
