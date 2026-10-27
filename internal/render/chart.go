package render

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vincentsch/startrust/internal/check"
	"github.com/vincentsch/startrust/internal/stars"
)

// MaxColumns is the most bars a chart draws. With the axis labels a chart
// fits in 80 terminal columns.
const MaxColumns = 64

// chartRows is the chart height in text lines. Each line holds eight
// levels, so bars have 64 steps.
const chartRows = 8

// Bucket is the stars in one bar of the chart.
type Bucket struct {
	Start time.Time
	End   time.Time // last day in the bucket
	Stars int
}

// Unit says how many days one bar covers.
type Unit struct {
	Name   string // "day", "week", "month", "2 months", "quarter", ...
	Days   int    // for day and week buckets
	Months int    // for month buckets
}

// ChooseUnit picks the smallest bucket that fits the series in MaxColumns
// bars: days for a young repo, then weeks, then months.
func ChooseUnit(s stars.Series) Unit {
	days := len(s.Daily)
	if days <= MaxColumns {
		return Unit{Name: "day", Days: 1}
	}
	if (days+6)/7 <= MaxColumns {
		return Unit{Name: "week", Days: 7}
	}
	months := monthsBetween(s.Start, s.End()) + 1
	for _, step := range []int{1, 2, 3, 4, 6, 12} {
		if (months+step-1)/step <= MaxColumns {
			return monthUnit(step)
		}
	}
	return monthUnit(24)
}

func monthUnit(step int) Unit {
	switch step {
	case 1:
		return Unit{Name: "month", Months: 1}
	case 3:
		return Unit{Name: "quarter", Months: 3}
	case 12:
		return Unit{Name: "year", Months: 12}
	}
	return Unit{Name: fmt.Sprintf("%d months", step), Months: step}
}

func monthsBetween(a, b time.Time) int {
	return (b.Year()-a.Year())*12 + int(b.Month()) - int(a.Month())
}

// MakeBuckets groups the series into bars. Week bars start on Monday; month
// bars start on the first of a month, so labels line up with the calendar.
func MakeBuckets(s stars.Series, unit Unit) []Bucket {
	if len(s.Daily) == 0 {
		return nil
	}
	var buckets []Bucket
	bucketStart := func(day time.Time) time.Time {
		switch {
		case unit.Months > 0:
			month := (int(day.Month()) - 1) / unit.Months * unit.Months
			if unit.Months >= 12 {
				month = 0
			}
			return time.Date(day.Year(), time.Month(month+1), 1, 0, 0, 0, 0, time.UTC)
		case unit.Days == 7:
			offset := (int(day.Weekday()) + 6) % 7 // days since Monday
			return day.AddDate(0, 0, -offset)
		default:
			return day
		}
	}
	for i, n := range s.Daily {
		day := s.Day(i)
		start := bucketStart(day)
		if len(buckets) == 0 || !buckets[len(buckets)-1].Start.Equal(start) {
			buckets = append(buckets, Bucket{Start: start})
		}
		last := &buckets[len(buckets)-1]
		last.Stars += n
		last.End = day
	}
	return buckets
}

// Marker labels one bar below the chart, such as a numbered spike.
type Marker struct {
	Day   time.Time
	Label string
}

// Chart draws a bar chart of stars over time with the given markers. The
// output uses plain Unicode blocks, so it reads the same on dark and light
// terminals and in Markdown code blocks.
func Chart(s stars.Series, markers []Marker) string {
	unit := ChooseUnit(s)
	buckets := MakeBuckets(s, unit)
	if len(buckets) == 0 {
		return ""
	}
	peak := 0
	for _, b := range buckets {
		peak = max(peak, b.Stars)
	}
	// Wider bars for short series, so a five-week repo is not a sliver.
	width := max(1, min(4, MaxColumns/len(buckets)))
	labelWidth := len(check.Number(peak))

	var out strings.Builder
	fmt.Fprintf(&out, "Stars per %s, %s to %s\n", unit.Name, periodLabel(buckets[0].Start, unit), periodLabel(buckets[len(buckets)-1].Start, unit))
	levels := make([]int, len(buckets))
	for i, b := range buckets {
		if peak > 0 {
			levels[i] = (b.Stars*chartRows*8 + peak - 1) / peak // round up so any star shows
		}
	}
	blocks := []rune(" ▁▂▃▄▅▆▇█")
	for row := chartRows - 1; row >= 0; row-- {
		label := ""
		if row == chartRows-1 {
			label = check.Number(peak)
		}
		axis := "│"
		if row == chartRows-1 {
			axis = "┤"
		}
		fmt.Fprintf(&out, "%*s %s", labelWidth, label, axis)
		var line strings.Builder
		for _, level := range levels {
			fill := min(8, max(0, level-row*8))
			line.WriteString(strings.Repeat(string(blocks[fill]), width))
		}
		out.WriteString(strings.TrimRight(line.String(), " ") + "\n")
	}

	// The time axis: a tick at each year (or month, or week for short
	// series). Calendar labels claim their space first, so a history that
	// starts in November still shows the next January; the start date is
	// in the title.
	ticks := []rune(strings.Repeat("─", len(buckets)*width))
	labels := newTextLine(len(buckets)*width + 12)
	var axis []axisLabel
	for i, b := range buckets {
		if text, priority, ok := tickLabel(b, i, unit); ok {
			if priority > 0 {
				ticks[i*width] = '┬'
			}
			axis = append(axis, axisLabel{col: i * width, text: text, priority: priority})
		}
	}
	sort.SliceStable(axis, func(i, j int) bool { return axis[i].priority > axis[j].priority })
	for _, l := range axis {
		labels.place(l.col, l.text)
	}
	fmt.Fprintf(&out, "%*s └%s\n", labelWidth, "", string(ticks))
	fmt.Fprintf(&out, "%*s  %s\n", labelWidth, "", labels.String())

	// Busy-day markers. Days in the same bar share one label ("2,4"), and
	// labels that would touch move to the next line, so every day in the
	// list has its number under the chart.
	var groups []axisLabel
	for _, m := range markers {
		col := -1
		for i, b := range buckets {
			if !m.Day.Before(b.Start) && !m.Day.After(b.End) {
				col = i*width + width/2
				break
			}
		}
		if col < 0 {
			continue
		}
		joined := false
		for i := range groups {
			if groups[i].col == col {
				groups[i].text += "," + m.Label
				joined = true
			}
		}
		if !joined {
			groups = append(groups, axisLabel{col: col, text: m.Label})
		}
	}
	var lines []*textLine
	for _, g := range groups {
		placed := false
		for _, line := range lines {
			if placed = line.place(g.col, g.text); placed {
				break
			}
		}
		if !placed {
			line := newTextLine(len(buckets)*width + 16)
			line.place(g.col, g.text)
			lines = append(lines, line)
		}
	}
	for _, line := range lines {
		fmt.Fprintf(&out, "%*s  %s\n", labelWidth, "", line.String())
	}
	return out.String()
}

func periodLabel(t time.Time, unit Unit) string {
	switch {
	case unit.Months >= 12:
		return t.Format("2006")
	case unit.Months > 0:
		return t.Format("2006-01")
	default:
		return t.Format("2006-01-02")
	}
}

// axisLabel is a label below the chart at a column. Labels with a higher
// priority are placed first.
type axisLabel struct {
	col      int
	text     string
	priority int
}

// textLine is one line of labels below the chart.
type textLine struct{ cells []rune }

func newTextLine(width int) *textLine { return &textLine{cells: []rune(strings.Repeat(" ", width))} }

// place writes text at col if it keeps a space to every label already on
// the line, and reports whether it did.
func (l *textLine) place(col int, text string) bool {
	runes := []rune(text)
	if col+len(runes) > len(l.cells) {
		return false
	}
	for c := max(0, col-1); c <= col+len(runes) && c < len(l.cells); c++ {
		if l.cells[c] != ' ' {
			return false
		}
	}
	copy(l.cells[col:], runes)
	return true
}

func (l *textLine) String() string { return strings.TrimRight(string(l.cells), " ") }

// tickLabel says whether bucket i gets a label on the time axis, and how
// much the label matters: 2 for a new year (or month for day bars), 1 for
// other calendar ticks, 0 for the start of the chart, which only shows when
// there is room.
func tickLabel(b Bucket, i int, unit Unit) (string, int, bool) {
	switch {
	case unit.Months > 0:
		if b.Start.Month() == time.January {
			return b.Start.Format("2006"), 2, true
		}
		if i == 0 {
			return b.Start.Format("2006"), 0, true
		}
	case unit.Days == 7:
		switch {
		case b.Start.Day() <= 7 && b.Start.Month() == time.January:
			return b.Start.Format("2006"), 2, true
		case b.Start.Day() <= 7:
			return b.Start.Format("Jan"), 1, true
		case i == 0:
			return b.Start.Format("Jan"), 0, true
		}
	default:
		switch {
		case b.Start.Day() == 1:
			return b.Start.Format("Jan 2"), 2, true
		case b.Start.Weekday() == time.Monday:
			return b.Start.Format("Jan 2"), 1, true
		case i == 0:
			return b.Start.Format("Jan 2"), 0, true
		}
	}
	return "", 0, false
}

// Sparkline draws stars over time as one line of block characters, scaled
// to its own peak.
func Sparkline(buckets []Bucket) string {
	peak := 0
	for _, b := range buckets {
		peak = max(peak, b.Stars)
	}
	blocks := []rune(" ▁▂▃▄▅▆▇█")
	var out strings.Builder
	for _, b := range buckets {
		level := 0
		if peak > 0 && b.Stars > 0 {
			level = max(1, (b.Stars*8+peak/2)/peak)
		}
		out.WriteRune(blocks[level])
	}
	return out.String()
}
