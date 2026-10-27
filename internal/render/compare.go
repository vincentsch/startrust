package render

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/vincentsch/startrust/internal/check"
	"github.com/vincentsch/startrust/internal/stars"
)

// Compare writes several results side by side: a table of the key numbers,
// their star history on one shared timeline, and each verdict.
func Compare(w io.Writer, results []*check.Result) {
	// The result column is the headline, which also says whether the
	// accounts were checked ("no unusual pattern" or "no unusual timing").
	rows := [][]string{{"Repo", "Stars", "Risk", "Result", "Biggest day"}}
	for _, r := range results {
		biggest := "-"
		if b := r.BiggestDay; b != nil {
			biggest = check.Number(b.Stars) + " on " + b.Day.Format("2006-01-02")
		}
		rows = append(rows, []string{r.Repository, check.Number(r.Stars), r.Risk, r.Headline, biggest})
	}
	widths := columnWidths(rows)
	for i, row := range rows {
		var line strings.Builder
		for col, cell := range row {
			pad := strings.Repeat(" ", widths[col]-len([]rune(cell)))
			if i > 0 && col == 1 {
				line.WriteString(pad + cell + "  ")
			} else {
				line.WriteString(cell + pad + "  ")
			}
		}
		fmt.Fprintln(w, strings.TrimRight(line.String(), " "))
	}

	if lines := sharedSparklines(results); len(lines) > 0 {
		fmt.Fprintln(w)
		for _, line := range lines {
			fmt.Fprintln(w, line)
		}
	}

	fmt.Fprintln(w)
	// The verdicts point at details below them in a single check; compare
	// has no such details, so that pointer goes.
	dropPointer := strings.NewReplacer(", see below.", ".", " See below.", "")
	for _, r := range results {
		fmt.Fprintln(w, wrap(r.Repository+": "+dropPointer.Replace(r.Verdict), 80, "  "))
	}
}

// sharedSparklines draws every repo's stars on the same calendar, each line
// scaled to its own busiest period.
func sharedSparklines(results []*check.Result) []string {
	var start, end time.Time
	for _, r := range results {
		if r.Details == nil || len(r.Details.History.Daily) == 0 {
			continue
		}
		h := r.Details.History
		if start.IsZero() || h.Start.Before(start) {
			start = h.Start
		}
		if h.End().After(end) {
			end = h.End()
		}
	}
	if start.IsZero() {
		return nil
	}
	span := stars.Series{Start: start, Daily: make([]int, int(end.Sub(start).Hours()/24)+1)}
	unit := chooseSparkUnit(span)
	nameWidth := 0
	for _, r := range results {
		nameWidth = max(nameWidth, len(r.Repository))
	}
	buckets := MakeBuckets(span, unit)
	lines := []string{fmt.Sprintf("Stars per %s, %s to %s (each line scaled to its own peak)",
		unit.Name, periodLabel(buckets[0].Start, unit), periodLabel(buckets[len(buckets)-1].Start, unit))}
	for _, r := range results {
		if r.Details == nil {
			continue
		}
		// Spread the repo's days over the shared calendar.
		padded := stars.Series{Start: start, Daily: make([]int, len(span.Daily))}
		h := r.Details.History
		for i, n := range h.Daily {
			if j := padded.Index(h.Day(i)); j >= 0 {
				padded.Daily[j] = n
			}
		}
		lines = append(lines, strings.TrimRight(fmt.Sprintf("%-*s │%s", nameWidth, r.Repository, Sparkline(MakeBuckets(padded, unit))), " "))
	}
	return lines
}

// chooseSparkUnit fits the shared timeline in about 50 columns, next to the
// repo names.
func chooseSparkUnit(s stars.Series) Unit {
	days := len(s.Daily)
	switch {
	case days <= 50:
		return Unit{Name: "day", Days: 1}
	case (days+6)/7 <= 50:
		return Unit{Name: "week", Days: 7}
	}
	months := monthsBetween(s.Start, s.End()) + 1
	for _, step := range []int{1, 2, 3, 4, 6, 12} {
		if (months+step-1)/step <= 50 {
			return monthUnit(step)
		}
	}
	return monthUnit(24)
}
