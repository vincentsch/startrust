package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/vincentsch/startrust/internal/check"
)

// Markdown writes a report for sharing, for example in a GitHub issue or an
// Actions job summary. The chart sits in a code block so the bars line up.
func Markdown(w io.Writer, r *check.Result, details bool) {
	fmt.Fprintf(w, "## Stars of [%s](%s)\n\n", r.Repository, r.URL)
	fmt.Fprintf(w, "**%s**\n\n", escape(r.Verdict))
	fmt.Fprintf(w, "%s stars, created %s. Risk: %s.\n\n", check.Number(r.Stars), r.CreatedAt.Format("2006-01-02"), r.RiskText())
	if line := formerNamesText(r); line != "" {
		fmt.Fprintf(w, "%s\n\n", escape(line))
	}

	fmt.Fprintln(w, "### Activity")
	fmt.Fprintln(w)
	for _, row := range activityRows(r) {
		fmt.Fprintf(w, "- %s\n", escape(strings.TrimSpace(row)))
	}
	fmt.Fprintln(w)
	if note := activityNote(r); note != "" {
		fmt.Fprintf(w, "%s\n\n", escape(note))
	}

	if chart := chartFor(r); chart != "" {
		fmt.Fprintf(w, "```text\n%s```\n\n", chart)
	}

	if len(r.Days) > 0 {
		fmt.Fprintln(w, "| # | Day | Stars | Vs normal day | Spike | Accounts |")
		fmt.Fprintln(w, "|---|-----|------:|--------------:|-------|----------|")
		for i, d := range r.Days {
			fmt.Fprintf(w, "| %d | %s | %s | %s | %s | %s |\n", i+1, d.Day.Format("2006-01-02"), check.Number(d.Stars),
				timesNormal(d.Ratio), dayLabel(r, i), escape(accountsText(d)))
		}
		fmt.Fprintln(w)
		for i, d := range r.Days {
			for _, e := range d.Explanations {
				line := explanationText(e)
				if e.URL != "" {
					line = fmt.Sprintf("[%s](%s)", escape(line), e.URL)
				}
				fmt.Fprintf(w, "- Day %d: %s\n", i+1, line)
			}
		}
		if hasExplanations(r) {
			fmt.Fprintln(w)
		}
		if note := sampleNote(r); note != "" {
			fmt.Fprintf(w, "%s\n\n", escape(note))
		}
		if note := explainNote(r); note != "" {
			fmt.Fprintf(w, "%s\n\n", escape(note))
		}
	}

	if len(r.Findings) > 0 {
		fmt.Fprintln(w, "### What stands out")
		fmt.Fprintln(w)
		for _, line := range r.Findings {
			fmt.Fprintf(w, "- %s\n", escape(line))
		}
		fmt.Fprintln(w)
	}

	if details {
		var buf strings.Builder
		writeDetails(&buf, r)
		fmt.Fprintf(w, "### Details\n\n```text\n%s\n```\n\n", strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(buf.String()), "Details")))
	}

	if len(r.Limits) > 0 {
		fmt.Fprintln(w, "### Limits")
		fmt.Fprintln(w)
		for _, line := range r.Limits {
			fmt.Fprintf(w, "- %s\n", escape(line))
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "<sub>Checked %s with [StarTrust](https://github.com/vincentsch/startrust).</sub>\n", r.CheckedAt.UTC().Format("2006-01-02 15:04 UTC"))
}

func hasExplanations(r *check.Result) bool {
	for _, d := range r.Days {
		if len(d.Explanations) > 0 {
			return true
		}
	}
	return false
}

// escape stops text from turning into Markdown markup or breaking tables.
func escape(s string) string {
	return strings.NewReplacer("|", "\\|", "*", "\\*", "_", "\\_", "<", "&lt;", ">", "&gt;", "`", "\\`", "[", "\\[", "]", "\\]").Replace(s)
}
