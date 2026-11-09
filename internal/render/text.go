// Package render turns check results into terminal text, Markdown and charts.
package render

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/vincentsch/startrust/internal/archive"
	"github.com/vincentsch/startrust/internal/check"
	"github.com/vincentsch/startrust/internal/stars"
)

// Text writes the one-screen report. With details it adds the full evidence.
func Text(w io.Writer, r *check.Result, details bool) {
	fmt.Fprintf(w, "%s · %s stars · created %s\n", r.Repository, check.Number(r.Stars), r.CreatedAt.Format("2006-01-02"))
	fmt.Fprintf(w, "%s\n\n", r.URL)
	fmt.Fprintln(w, wrap(r.Verdict, 80, ""))
	fmt.Fprintf(w, "Risk: %s\n", r.Risk)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Activity")
	for _, row := range activityRows(r) {
		fmt.Fprintln(w, "  "+row)
	}
	if note := activityNote(r); note != "" {
		fmt.Fprintln(w, wrap("  "+note, 80, "  "))
	}

	if chart := chartFor(r); chart != "" {
		fmt.Fprintln(w)
		fmt.Fprint(w, chart)
	}

	if len(r.Days) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Busiest days, at least a week apart")
		writeDays(w, r)
		if note := explainNote(r); note != "" {
			fmt.Fprintln(w, wrap("  "+note, 80, "  "))
		}
	}

	if len(r.Findings) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "What stands out")
		for _, line := range r.Findings {
			fmt.Fprintln(w, wrap("- "+line, 80, "  "))
		}
	}

	if details {
		writeDetails(w, r)
	}

	// A repo without stars has no limits worth a heading.
	if len(r.Limits) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Limits")
		for _, line := range r.Limits {
			fmt.Fprintln(w, wrap("- "+line, 80, "  "))
		}
	}

	fmt.Fprintln(w)
	footer := fmt.Sprintf("Checked %s using %s.", r.CheckedAt.Local().Format("2006-01-02 15:04 MST"), requestsText(r))
	if token := tokenText(r.Sources.TokenSource); token != "" {
		footer += " " + token
	}
	if !details {
		footer += " Add --details for the full evidence."
	}
	fmt.Fprintln(w, wrap(footer, 80, ""))
}

// activityRows lists what people do with the repo besides starring it,
// with the counts right-aligned.
func activityRows(r *check.Result) []string {
	forks := word(r.Forks, "fork", "forks")
	if r.Stars >= check.ActivityMinStars {
		forks += " (" + r.ForksPer1000Text() + " per 1,000 stars)"
	}
	// When the search did not answer, the limits say why.
	issues := [2]string{"?", "issues and pull requests " + r.IssuesPeriod() + " (not counted)"}
	if n := r.IssuesAndPRs; n != nil {
		issues = [2]string{check.Number(*n), word(*n, "issue or pull request", "issues and pull requests") + " " + r.IssuesPeriod()}
	}
	rows := [][2]string{
		{check.Number(r.Forks), forks},
		{check.Number(r.Watchers), word(r.Watchers, "watcher", "watchers")},
		issues,
	}
	width := 0
	for _, row := range rows {
		width = max(width, len(row[0]))
	}
	lines := make([]string, len(rows))
	for i, row := range rows {
		lines[i] = strings.Repeat(" ", width-len(row[0])) + row[0] + " " + row[1]
	}
	return lines
}

// activityNote says how to read the activity. A repo with few stars gets
// none: its forks per 1,000 stars swing too much to compare.
func activityNote(r *check.Result) string {
	if r.Stars < check.ActivityMinStars {
		return ""
	}
	return check.ActivityNote()
}

func chartFor(r *check.Result) string {
	// A repo without stars gets no chart: eight empty lines say nothing.
	if r.Details == nil || r.Details.History.Total() == 0 {
		return ""
	}
	var markers []Marker
	for i, d := range r.Days {
		markers = append(markers, Marker{Day: d.Day, Label: fmt.Sprint(i + 1)})
	}
	return Chart(r.Details.History, markers)
}

func writeDays(w io.Writer, r *check.Result) {
	rows := [][]string{}
	for i, d := range r.Days {
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			d.Day.Format("2006-01-02"),
			check.Number(d.Stars) + " stars",
			timesNormal(d.Ratio),
			dayLabel(r, i),
			accountsText(d),
		})
	}
	widths := columnWidths(rows)
	for i, row := range rows {
		var line strings.Builder
		line.WriteString("  ")
		for col, cell := range row {
			if col == 2 || col == 3 {
				line.WriteString(strings.Repeat(" ", widths[col]-len([]rune(cell))) + cell + "  ")
			} else {
				line.WriteString(cell + strings.Repeat(" ", widths[col]-len([]rune(cell))) + "  ")
			}
		}
		fmt.Fprintln(w, strings.TrimRight(line.String(), " "))
		for _, e := range r.Days[i].Explanations {
			fmt.Fprintln(w, wrap("     "+explanationText(e), 80, "     "))
		}
	}
}

func columnWidths(rows [][]string) []int {
	var widths []int
	for _, row := range rows {
		for col, cell := range row {
			if col >= len(widths) {
				widths = append(widths, 0)
			}
			widths[col] = max(widths[col], len([]rune(cell)))
		}
	}
	return widths
}

func timesNormal(ratio float64) string { return check.RatioText(ratio) + "x normal" }

// dayLabel names the spike a listed day belongs to. A day inside a spike
// whose biggest day is also listed points at that day, so one spike never
// looks like two. Saved summaries have no spike list and get the plain
// label.
func dayLabel(r *check.Result, i int) string {
	d := r.Days[i]
	if r.Details != nil && d.Spike != "" {
		for _, spike := range r.Details.Spikes {
			if d.Day.Before(spike.Start) || d.Day.After(spike.End) || d.Day.Equal(spike.Peak) {
				continue
			}
			for j, other := range r.Days {
				if other.Day.Equal(spike.Peak) {
					return fmt.Sprintf("part of %d", j+1)
				}
			}
		}
	}
	return spikeLabel(d.Spike)
}

func spikeLabel(spike string) string {
	switch spike {
	case "unusual":
		return "unusual spike"
	case "launch":
		return "launch"
	case "spike":
		return "spike"
	}
	return ""
}

func accountsText(d check.Day) string {
	var parts []string
	if c := d.Accounts; c != nil {
		switch {
		case !c.Usable():
			parts = append(parts, "too few in GH Archive")
		case c.Provisional:
			parts = append(parts, fmt.Sprintf("%d%% so far (provisional)", int(math.Round(c.Share()*100))))
		case c.Checked < check.MinSample:
			// A share of a small sample shows its counts, so nobody reads
			// 30% of 30 accounts like 30% of 1,000.
			parts = append(parts, fmt.Sprintf("%s of %s one-off", check.Number(c.OneOff), check.Number(c.Checked)))
		default:
			parts = append(parts, fmt.Sprintf("%d%% one-off accounts", int(math.Round(c.Share()*100))))
		}
	}
	if p := d.Profiles; p != nil && p.Accounts > 0 {
		parts = append(parts, fmt.Sprintf("%d%% new or empty profiles", int(math.Round(100*float64(p.NewOrEmpty)/float64(p.Accounts)))))
	}
	return strings.Join(parts, ", ")
}

// explainNote says when --explain ran but found nothing to show, so the
// silence does not look like a skipped lookup.
func explainNote(r *check.Result) string {
	if hasExplanations(r) {
		return ""
	}
	switch r.Sources.Explain {
	case "used":
		return "--explain found no Hacker News posts or releases from these days or the two days before."
	case "partial":
		return "--explain found no Hacker News posts or releases from these days or the two days before. " +
			"Part of the lookup failed (" + r.Sources.ExplainProblem + ")."
	}
	return ""
}

func explanationText(e check.Explanation) string {
	switch e.Kind {
	case "hacker_news":
		return fmt.Sprintf("Hacker News on %s: %q (%s points)", e.Day.Format("2006-01-02"), e.Title, check.Number(e.Points))
	case "release":
		return fmt.Sprintf("Release %s on %s", e.Title, e.Day.Format("2006-01-02"))
	}
	return e.Title
}

func requestsText(r *check.Result) string {
	text := check.Number(r.Sources.GitHubRequests) + " GitHub requests"
	if r.Sources.GitHubRequests == 1 {
		text = "1 GitHub request"
	}
	switch r.Sources.ArchiveQueries {
	case 0:
	case 1:
		text += " and 1 GH Archive query"
	default:
		text += fmt.Sprintf(" and %d GH Archive queries", r.Sources.ArchiveQueries)
	}
	return text
}

// tokenText says which GitHub token the check used. A token can come from
// a place the user forgot about, such as a GitHub CLI login in the system
// keyring, so the report always names it.
func tokenText(source string) string {
	switch source {
	case "GITHUB_TOKEN", "GH_TOKEN":
		return "Used the token in " + source + "."
	case "gh":
		return "Used your GitHub CLI login, 5,000 requests an hour."
	case "none":
		return "Used no GitHub token, 60 requests an hour."
	}
	return ""
}

func writeDetails(w io.Writer, r *check.Result) {
	d := r.Details
	// line prints one wrapped line of the details: indent for the first
	// line, and two more spaces for the lines it wraps onto.
	line := func(indent, format string, args ...any) {
		fmt.Fprintln(w, wrap(indent+fmt.Sprintf(format, args...), 80, indent+"  "))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Details")
	line("  ", "Star history: %s to %s, %s stars counted (GitHub shows %s now)",
		r.HistoryFrom.Format("2006-01-02"), r.HistoryTo.Format("2006-01-02"), check.Number(totalOf(d)), check.Number(r.Stars))
	if d != nil {
		rate := "about " + formatRate(d.RecentDaily) + " stars a day"
		if d.RecentDaily == 0 {
			rate = "no stars"
		}
		if days := len(d.History.Daily); days < 30 {
			line("  ", "Since %s (%s): %s", d.History.Start.Format("2006-01-02"), plural(days, "day", "days"), rate)
		} else {
			line("  ", "Last 30 days: %s", rate)
		}
	}
	if d != nil && len(d.Spikes) > 0 {
		fmt.Fprintln(w)
		line("  ", "Spikes (days with at least %d stars and %gx a normal day)", stars.SpikeMinStars, stars.SpikeRatio)
		for _, s := range d.Spikes {
			span := s.Start.Format("2006-01-02")
			if !s.End.Equal(s.Start) {
				span += " to " + s.End.Format("2006-01-02")
			}
			text := fmt.Sprintf("%s: peak %s on %s, %s in all, %s", span, check.Number(s.PeakStars), s.Peak.Format("Jan 2"),
				check.Number(s.Stars), timesNormal(s.Ratio))
			switch {
			case s.Unusual:
				text += ", unusual"
			case s.Launch:
				text += ", launch"
			}
			line("    ", "%s", text)
		}
	}
	if b := r.Burst; b != nil {
		fmt.Fprintln(w)
		line("  ", "Burst: %s to %s, %s stars (%d%% of all)", b.Start.Format("2006-01-02"), b.End.Format("2006-01-02"),
			check.Number(b.Stars), int(math.Round(b.Share*100)))
		line("    ", "Stars a day before: %s, during: %s, last 90 days: %s", formatRate(b.DailyBefore), formatRate(b.DailyDuring), formatRate(b.DailyAfter))
	}
	hasAccounts := false
	for _, day := range r.Days {
		if day.Accounts != nil {
			hasAccounts = true
		}
	}
	if hasAccounts {
		fmt.Fprintln(w)
		line("  ", "GH Archive accounts (one-off: no other public star within %d days before or after)", archive.WindowDays)
		for _, day := range r.Days {
			c := day.Accounts
			if c == nil {
				continue
			}
			text := fmt.Sprintf("%s: GitHub counts %s stars, GH Archive has %s accounts",
				day.Day.Format("2006-01-02"), check.Number(c.GitHubStars), check.Number(c.Archived))
			switch {
			case c.Gap():
				text += ", not judged (GH Archive missed many stars around this day)"
			case !c.Usable():
				text += ", too few to judge"
			case c.Provisional:
				text += fmt.Sprintf(", %s checked, %s one-off so far (provisional)", check.Number(c.Checked), check.Number(c.OneOff))
			default:
				text += fmt.Sprintf(", %s checked, %s one-off", check.Number(c.Checked), check.Number(c.OneOff))
			}
			line("    ", "%s", text)
		}
	}
	if d != nil && d.Profiles != nil {
		p := d.Profiles
		fmt.Fprintln(w)
		line("  ", "Stargazer profiles: %s listed, %s new (created at most %d days before starring), "+
			"%s with no followers and no public repositories, %s either or both",
			check.Number(p.Accounts), check.Number(p.New), check.NewAccountDays, check.Number(p.Empty), check.Number(p.NewOrEmpty))
	}
	s := r.Sources
	fmt.Fprintln(w)
	line("  ", "Sources: star history %s, GH Archive %s, stargazer list %s, explanations %s",
		sourceWords["history "+s.History], sourceWords["archive "+s.Archive], sourceWords["stargazers "+s.Stargazers], sourceWords["explain "+s.Explain])
	if s.ExplainProblem != "" {
		line("  ", "Explanations incomplete: %s", s.ExplainProblem)
	}
}

// sourceWords turns the source states of the JSON into words.
var sourceWords = map[string]string{
	"history complete":    "complete",
	"history partial":     "partly read",
	"history unavailable": "unavailable",
	"archive used":        "used",
	"archive partial":     "partly used",
	"archive unavailable": "unavailable",
	"archive not_needed":  "not needed (no busy day)",
	"archive off":         "off (--no-archive)",
	"stargazers listed":   "read",
	"stargazers partial":  "partly read",
	"stargazers hidden":   "hidden by GitHub",
	"stargazers off":      "not read (needs --deep)",
	"explain used":        "used",
	"explain partial":     "partly found",
	"explain off":         "off (no --explain)",
}

func word(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return check.Number(n) + " " + many
}

func totalOf(d *check.Details) int {
	if d == nil {
		return 0
	}
	return d.History.Total()
}

func formatRate(perDay float64) string {
	switch {
	case perDay == 0:
		return "0"
	case perDay < 0.05:
		return "under 0.1"
	case perDay < 10:
		return fmt.Sprintf("%.1f", perDay)
	}
	return check.Number(int(math.Round(perDay)))
}

// Wrap breaks text into lines of at most width runes; continuation lines
// start with indent.
func Wrap(text string, width int, indent string) string { return wrap(text, width, indent) }

// wrap breaks text into lines of at most width runes; continuation lines
// start with indent.
func wrap(text string, width int, indent string) string {
	// Leading spaces count toward the first line's width.
	lead := text[:len(text)-len(strings.TrimLeft(text, " "))]
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "" && len(lines) == 0:
			line = lead + word
		case line == "":
			line = indent + word
		case len([]rune(line))+1+len([]rune(word)) > width:
			lines = append(lines, line)
			line = indent + word
		default:
			line += " " + word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
