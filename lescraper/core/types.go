package core

import "time"

// Status is the ticket status of a show, as far as the listing says.
type Status string

const (
	StatusOnSale     Status = "on_sale"
	StatusSoldOut    Status = "sold_out"
	StatusComingSoon Status = "coming_soon"
	StatusUnknown    Status = "unknown"
)

// Show represents a concert/show at a venue
type Show struct {
	Title string // Raw listing text (may contain multiple artists)
	Venue string // Venue name as configured in venues.yaml (e.g., "Webster Hall")
	Room  string // Room within the venue when it has several (e.g., "Garcia's"); empty otherwise

	// Date is the show's calendar date at midnight, America/New_York.
	Date time.Time
	// Time is the listed start time ("19:00", 24h, New York time); empty if unknown.
	// Some sources only list doors, in which case it is the doors time.
	Time string

	// Headliner and Support are set when the source lists the lineup explicitly.
	// When Headliner is empty, use Lineup() to split Title best-effort.
	Headliner string
	Support   []string

	Status   Status
	OnSaleAt time.Time // when general sale starts/started; zero if unknown

	Price    string
	URL      string // ticket or event page
	VenueURL string
}

// NormalizedShow represents a show with cleaned and extracted artist information
type NormalizedShow struct {
	Artists  []string // Multiple artists extracted from the show title
	Venue    string
	Date     time.Time
	Price    string
	URL      string
	VenueURL string
}

// Scraper is the interface that all venue scrapers must implement.
// Each registered venue name has its own Scraper instance, so a failure is
// reported against that venue only.
type Scraper interface {
	// GetVenues returns the venue names this scraper provides
	GetVenues() []string

	// ScrapeShows scrapes every upcoming show the venue lists. Results set
	// Show.Venue to the registered venue name. Date-window filtering is done
	// by the caller.
	ScrapeShows() ([]Show, error)

	// GetName returns a human-readable name for this scraper
	GetName() string

	// GetURL returns the human-facing listings page for the venue
	GetURL() string
}
