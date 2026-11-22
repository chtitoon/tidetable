package core

import (
	"reflect"
	"testing"
)

// Real titles from the venues' listings.
func TestSplitLineup(t *testing.T) {
	tests := []struct {
		title     string
		headliner string
		support   []string
	}{
		{"Friendship & Charlotte Cornfield", "Friendship", []string{"Charlotte Cornfield"}},
		{"Steve Gunn & Bridget St John", "Steve Gunn", []string{"Bridget St John"}},
		{"Hello Dreamer, ELIS, Audrey Mason", "Hello Dreamer", []string{"ELIS", "Audrey Mason"}},
		{"Del Coda, Off the Hook, Dylan Taganas, and more!", "Del Coda", []string{"Off the Hook", "Dylan Taganas"}},
		{"Emmett & the Dead Ends w/ Dog House", "Emmett & the Dead Ends", []string{"Dog House"}},
		{"Dust Jacket w/ Joe’s Fired", "Dust Jacket", []string{"Joe’s Fired"}},
		// "X & The Ys" and one-word duos are band names, not two acts.
		{"Annalise & The Mother Underground", "Annalise & The Mother Underground", []string{}},
		{"Milk & Bone", "Milk & Bone", []string{}},
		// Lowercase "and" inside a title doesn't split.
		{"Big Bad Voodoo Daddy's Wild and Swingin’ Holiday Party", "Big Bad Voodoo Daddy's Wild and Swingin’ Holiday Party", []string{}},
		// Subtitles go; a series name before a list keeps the list.
		{"Darlene Love: Love For The Holidays", "Darlene Love", []string{}},
		{"SUM SUN: Rendezvous Album Release Show", "SUM SUN", []string{}},
		{"Joe Jordan: The Heart Sessions Tour", "Joe Jordan", []string{}},
		{"Cherish the Ladies - A Celtic Christmas", "Cherish the Ladies", []string{}},
		{"Aged Inventory – The Concert for Parkinson’s Research", "Aged Inventory", []string{}},
		{"Yuki Chiba / FIRST NORTH AMERICA HEADLINE TOUR", "Yuki Chiba", []string{}},
		{"The Well: Paul Babe, John Dorney, Lucas Lusher, and Besche", "Paul Babe", []string{"John Dorney", "Lucas Lusher", "Besche"}},
		{"M.E.A.T. x Webster Hall - Adham + Suri + More", "Adham", []string{"Suri"}},
		{"An Evening with Richard Marx", "Richard Marx", []string{}},
		{"Fantastic Cat w/ very special guest Low Cut Connie (solo) 107.1 The Peak’s Holiday Hang", "Fantastic Cat", []string{"Low Cut Connie"}},
		{"Lucy Woodward & Juliet Lloyd", "Lucy Woodward", []string{"Juliet Lloyd"}},
		{"Triple Charm", "Triple Charm", []string{}},
	}
	for _, tt := range tests {
		h, s := Lineup(Show{Title: tt.title})
		if h != tt.headliner || !reflect.DeepEqual(s, tt.support) {
			t.Errorf("Lineup(%q) = %q, %q; want %q, %q", tt.title, h, s, tt.headliner, tt.support)
		}
	}
}

func TestLineupExplicit(t *testing.T) {
	// Explicit lineup from the source wins; support is cleaned and deduped.
	h, s := Lineup(Show{
		Title:     "An Evening With Sasha Alex Sloan: And The Crowd Goes Mild Tour",
		Headliner: "Sasha Alex Sloan",
		Support:   []string{"With Special Guest Macy Todd", "sasha alex sloan", ""},
	})
	if h != "Sasha Alex Sloan" || !reflect.DeepEqual(s, []string{"Macy Todd"}) {
		t.Errorf("got %q, %q", h, s)
	}

	// Explicit support with no explicit headliner merges with the title split
	// (Mercury East lists "with X, Y" separately and repeats some in the name).
	h, s = Lineup(Show{
		Title:   "misc., Red Girl’s Blue, Circle of Sevens",
		Support: []string{"Red Girl’s Blue", "Circle of Sevens"},
	})
	if h != "misc." || !reflect.DeepEqual(s, []string{"Red Girl’s Blue", "Circle of Sevens"}) {
		t.Errorf("got %q, %q", h, s)
	}
}

func TestIsNonConcert(t *testing.T) {
	drop := []string{
		"DJ Dance Party", "Emo Night Brooklyn", "Emo Nite", "Live Band Karaoke",
		"FREE 4:30 PM Movie", "Film Series: It's A Wonderful Life", "Annie The Musical -Opens Nov 21!-",
		"Christmas Eve Dinner at The Grand", "Makers Market Brunch", "Twiddle 3 DAY PASS",
		"Annie The Musical SCHOOL SHOW", "Parking M Bamford 11/21/26 - CENTRAL AVE LOT",
		"$20 Post-Show M&G add-on NOT A TICKET", "Live Talks NY presents Michael Lewis",
		"Westchester Ballet Company presents The Nutcracker",
	}
	keep := []string{
		"Live Music Brunch", "Big Bad Voodoo Daddy's Wild and Swingin’ Holiday Party",
		"DJ Mandy", "The Machine Performs Pink Floyd", "Bluegrass Night with Bigdraw & Friends",
		"Songwriters Circle Holiday Showcase", "THANKSGIVING EVE",
	}
	for _, title := range drop {
		if !IsNonConcert(title) {
			t.Errorf("IsNonConcert(%q) = false, want true", title)
		}
	}
	for _, title := range keep {
		if IsNonConcert(title) {
			t.Errorf("IsNonConcert(%q) = true, want false", title)
		}
	}
}

func TestIsPlaceholderName(t *testing.T) {
	for _, name := range []string{
		"@", " @ ", "?", "X", "--", "DJ", "TBA", "tba", "TBD", "T.B.A.", "To be announced", "To Be Determined",
		"Special Guest", "Special Guests", "special guest(s)", "Very Special Guests!", "Special Guests TBA",
	} {
		if !IsPlaceholderName(name) {
			t.Errorf("IsPlaceholderName(%q) = false, want true", name)
		}
	}
	// Short real names with 3+ letters or digits, and names that merely
	// contain a placeholder word, are real acts.
	for _, name := range []string{
		"MGMT", "IDLES", "Wet", "Low", "S.C.U.M", "311", "Björk", "The Special Guests Band", "TBA Rhythm Section",
	} {
		if IsPlaceholderName(name) {
			t.Errorf("IsPlaceholderName(%q) = true, want false", name)
		}
	}
}

func TestLineupDropsPlaceholderSupport(t *testing.T) {
	h, s := Lineup(Show{Title: "IDLES", Support: []string{"TBA", "@", "Special Guest(s)", "Squid", "To Be Announced"}})
	if h != "IDLES" || !reflect.DeepEqual(s, []string{"Squid"}) {
		t.Errorf("got %q, %q; want IDLES, [Squid]", h, s)
	}
	// Placeholders inside a split title go too.
	h, s = Lineup(Show{Title: "Whitney w/ TBA"})
	if h != "Whitney" || len(s) != 0 {
		t.Errorf("got %q, %q; want Whitney, []", h, s)
	}
}
