// Package stars finds spikes and bursts in GitHub's daily star counts. It
// does no I/O, so every result can be checked in tests.
package stars

import (
	"sort"
	"time"
	// GitHub's calendar is US Pacific time. Embedding the zone data makes
	// that also work on Windows and in minimal containers.
	_ "time/tzdata"

	"github.com/vincentsch/startrust/internal/github"
)

// Series holds the stars a repo received on each day, from Start to the last
// day GitHub reported. Days are GitHub's calendar days, which follow US
// Pacific time. A Time in a Series is always midnight UTC of that date.
type Series struct {
	Start time.Time `json:"start"`
	Daily []int     `json:"daily"`
}

// Day returns the date of day i.
func (s Series) Day(i int) time.Time { return s.Start.AddDate(0, 0, i) }

// End returns the date of the last day.
func (s Series) End() time.Time { return s.Day(len(s.Daily) - 1) }

// Index returns the position of a date in the series, or -1.
func (s Series) Index(day time.Time) int {
	i := int(day.Sub(s.Start).Hours() / 24)
	if i < 0 || i >= len(s.Daily) {
		return -1
	}
	return i
}

// Total returns the number of stars in the series.
func (s Series) Total() int { return sum(s.Daily) }

// Stars returns the stars on a date, or 0 outside the series.
func (s Series) Stars(day time.Time) int {
	if i := s.Index(day); i >= 0 {
		return s.Daily[i]
	}
	return 0
}

// FromWeeks builds a series from GitHub's weekly history rows. Days before
// the repo was created and after today are dropped. Today is included even
// though it is not over yet.
func FromWeeks(weeks []github.Week, created, today time.Time) Series {
	if len(weeks) == 0 {
		return Series{}
	}
	sorted := append([]github.Week(nil), weeks...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Week < sorted[j].Week })

	first := Date(time.Unix(sorted[0].Week, 0))
	last := Date(time.Unix(sorted[len(sorted)-1].Week, 0)).AddDate(0, 0, 6)
	today = Date(today)
	if last.After(today) {
		last = today
	}
	start := Date(created)
	if start.Before(first) {
		start = first
	}
	// A repo can hold stars from before its creation date when it was
	// transferred or recreated. Never drop counted stars.
	for _, week := range sorted {
		for d, n := range week.Days {
			day := Date(time.Unix(week.Week, 0)).AddDate(0, 0, d)
			if n > 0 && day.Before(start) {
				start = day
			}
		}
	}
	if last.Before(start) {
		return Series{Start: start}
	}

	days := int(last.Sub(start).Hours()/24) + 1
	series := Series{Start: start, Daily: make([]int, days)}
	for _, week := range sorted {
		for d, n := range week.Days {
			if i := series.Index(Date(time.Unix(week.Week, 0)).AddDate(0, 0, d)); i >= 0 {
				series.Daily[i] += n
			}
		}
	}
	return series
}

// Date truncates a time to midnight UTC of its UTC date.
func Date(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// pacific is GitHub's calendar for star history.
var pacific = loadPacific()

func loadPacific() *time.Location {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.FixedZone("PST", -8*3600)
	}
	return loc
}

// Today returns today's date on GitHub's calendar.
func Today(now time.Time) time.Time {
	local := now.In(pacific)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

func sum(values []int) int {
	total := 0
	for _, v := range values {
		total += v
	}
	return total
}

// Median returns the middle value (the mean of the two middle values for an
// even count), or 0 for no values.
func Median(values []int) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return float64(sorted[mid])
	}
	return float64(sorted[mid-1]+sorted[mid]) / 2
}
