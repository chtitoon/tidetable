package core

import "regexp"

// nonConcertPattern matches titles that are obviously not concerts. It is
// deliberately conservative: when unsure, keep the show. Scrapers whose
// source flags events (genre, segment, category) filter on that first; this
// catches what's left, e.g. on venues that only publish a title.
var nonConcertPattern = regexp.MustCompile(`(?i)` +
	`\bdj (?:night|party|dance party)\b|\bdance party\b|\bemo ni(?:ght|te)\b|\bsilent disco\b` +
	`|\bkaraoke\b|\btrivia\b|\bbingo\b` +
	`|\bcomedy\b|\bcomedian\b|\bstand-?up\b|\bpodcast\b|\blive talks?\b` +
	`|\bthe musical\b|\bnutcracker\b|\bballet\b` +
	`|\bfilm series\b|\bmovie\b|\bscreening\b` +
	`|\bdinner\b|\bmakers market\b|\bmarket brunch\b` +
	`|\bschool show\b|\bparking\b|\bnot a (?:concert )?ticket\b|\badd-on\b|\b(?:\d+[- ])?day pass\b` +
	`|\bprivate event\b|\bclosed\b`)

// IsNonConcert reports whether a show title is obviously not a concert
// (DJ nights, comedy, talks, musicals, films, dinners, parking, passes...).
func IsNonConcert(title string) bool {
	return nonConcertPattern.MatchString(title)
}
