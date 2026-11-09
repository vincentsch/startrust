package render

import (
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/startrust/internal/archive"
	"github.com/vincentsch/startrust/internal/check"
	"github.com/vincentsch/startrust/internal/stars"
)

func TestRepoWithoutStarsHasNoChart(t *testing.T) {
	created := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	r := &check.Result{Repository: "me/new", CreatedAt: created, Details: &check.Details{
		History: stars.Series{Start: created, Daily: make([]int, 26)},
	}}
	check.Decide(r, stars.Analyze(r.Details.History))
	var out strings.Builder
	Text(&out, r, false)
	if strings.Contains(out.String(), "Stars per") || !strings.Contains(out.String(), "No stars yet") {
		t.Errorf("output:\n%s", out.String())
	}
	// Such a repo has no limits, so it gets no empty Limits heading.
	var markdown strings.Builder
	Markdown(&markdown, r, false)
	if strings.Contains(out.String(), "Limits") || strings.Contains(markdown.String(), "Limits") {
		t.Errorf("empty Limits heading:\n%s\n%s", out.String(), markdown.String())
	}
}

func TestYoungRepoDetailsSayHowManyDays(t *testing.T) {
	created := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	daily := make([]int, 26)
	daily[5] = 3000
	r := &check.Result{Repository: "me/new", Stars: 3000, CreatedAt: created, Details: &check.Details{
		History: stars.Series{Start: created, Daily: daily}, RecentDaily: 3000.0 / 26,
	}}
	var out strings.Builder
	Text(&out, r, true)
	if !strings.Contains(out.String(), "Since 2026-10-01 (26 days): about 115 stars a day") || strings.Contains(out.String(), "Last 30 days") {
		t.Errorf("output:\n%s", out.String())
	}
	// A quiet month says so in words.
	r.Details.RecentDaily = 0
	out.Reset()
	Text(&out, r, true)
	if !strings.Contains(out.String(), "Since 2026-10-01 (26 days): no stars") {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestSmallSamplesShowTheirCounts(t *testing.T) {
	small := check.Day{Accounts: &archive.Cohort{Archived: 30, Checked: 30, OneOff: 9}}
	big := check.Day{Accounts: &archive.Cohort{Archived: 1000, Checked: 1000, OneOff: 90}}
	if got := accountsText(small); got != "9 of 30 one-off" {
		t.Errorf("small sample: %q", got)
	}
	if got := accountsText(big); got != "9% one-off accounts" {
		t.Errorf("big sample: %q", got)
	}
}

func TestSampledSharesSaySo(t *testing.T) {
	// GH Archive had 1,616 accounts, and StarTrust checked 1,000 of them.
	day := time.Date(2022, 7, 29, 0, 0, 0, 0, time.UTC)
	sampled := check.Day{Day: day, Stars: 1566, Accounts: &archive.Cohort{Day: day, GitHubStars: 1566, Archived: 1616, Checked: 1000, OneOff: 80}}
	if got := accountsText(sampled); got != "8% one-off (sample)" {
		t.Errorf("sampled day: %q", got)
	}
	r := &check.Result{Repository: "a/b", Stars: 1566, Days: []check.Day{sampled}, Details: &check.Details{}}
	var out strings.Builder
	Text(&out, r, true)
	text := strings.Join(strings.Fields(out.String()), " ")
	for _, want := range []string{
		"On days marked (sample), GH Archive had more than 1,000 accounts, so StarTrust checked 1,000 of them.",
		"GH Archive has 1,616 accounts, a sample of 1,000 checked, 80 one-off",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the report does not say %q:\n%s", want, out.String())
		}
	}
	var markdown strings.Builder
	Markdown(&markdown, r, false)
	if !strings.Contains(markdown.String(), "On days marked (sample)") {
		t.Errorf("the Markdown report does not explain the sample:\n%s", markdown.String())
	}
	// A day with every account checked gets no note.
	sampled.Accounts.Archived = 1000
	out.Reset()
	Text(&out, r, false)
	if strings.Contains(out.String(), "sample") {
		t.Errorf("a day without a sample mentions one:\n%s", out.String())
	}
}

func TestActivityShowsWhatIsKnown(t *testing.T) {
	checked := time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC)
	issues, one := 142, 1
	young := &check.Result{Stars: 20112, Forks: 1971, Watchers: 150, IssuesAndPRs: &issues,
		CreatedAt: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), CheckedAt: checked}
	want := []string{
		"1,971 forks (98 per 1,000 stars)",
		"  150 watchers",
		"  142 issues and pull requests so far",
	}
	if got := activityRows(young); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("young repo:\n%s", strings.Join(got, "\n"))
	}
	if note := activityNote(young); !strings.Contains(note, "at least 10 forks per 1,000 stars") ||
		!strings.Contains(note, "Few forks alone also mean nothing, because tools such as `composer thanks` star library repos automatically.") {
		t.Errorf("note = %q", activityNote(young))
	}

	// A small repo gets no ratio and no note: a fork more or less would
	// swing the ratio. An older repo counts the last 90 days, and a count
	// the search could not get says so (the limits say why).
	small := &check.Result{Stars: 120, Forks: 1, Watchers: 1, IssuesAndPRs: &one,
		CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), CheckedAt: checked}
	want = []string{
		"1 fork",
		"1 watcher",
		"1 issue or pull request in the last 90 days",
	}
	if got := activityRows(small); strings.Join(got, "\n") != strings.Join(want, "\n") || activityNote(small) != "" {
		t.Errorf("small repo:\n%s\nnote %q", strings.Join(got, "\n"), activityNote(small))
	}
	small.IssuesAndPRs = nil
	if got := activityRows(small)[2]; got != "? issues and pull requests in the last 90 days (not counted)" {
		t.Errorf("unknown count: %q", got)
	}
}

func TestExplainSaysWhenItFoundNothing(t *testing.T) {
	r := &check.Result{Repository: "me/tool", Days: []check.Day{{Day: time.Date(2025, 1, 28, 0, 0, 0, 0, time.UTC), Stars: 900}},
		Sources: check.Sources{Explain: "used"}}
	var out, markdown strings.Builder
	Text(&out, r, false)
	Markdown(&markdown, r, false)
	// The text view wraps the note, so compare words.
	flat := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	want := "--explain found no Hacker News posts or releases from these days or the two days before."
	if !strings.Contains(flat(out.String()), want) || !strings.Contains(markdown.String(), want) {
		t.Errorf("text:\n%s\nmarkdown:\n%s", out.String(), markdown.String())
	}

	r.Sources.Explain, r.Sources.ExplainProblem = "partial", "cannot reach Hacker News search: timeout"
	out.Reset()
	Text(&out, r, false)
	if !strings.Contains(flat(out.String()), "Part of the lookup failed (cannot reach Hacker News search: timeout).") {
		t.Errorf("partial lookup:\n%s", out.String())
	}

	// Without --explain, or when something was found, there is no note.
	r.Sources.Explain = "off"
	out.Reset()
	Text(&out, r, false)
	if strings.Contains(out.String(), "--explain found") {
		t.Errorf("note without --explain:\n%s", out.String())
	}
	r.Sources.Explain = "used"
	r.Days[0].Explanations = []check.Explanation{{Kind: "release", Day: r.Days[0].Day, Title: "v1.0.0"}}
	out.Reset()
	Text(&out, r, false)
	if strings.Contains(out.String(), "--explain found") || !strings.Contains(out.String(), "Release v1.0.0 on 2025-01-28") {
		t.Errorf("found a release:\n%s", out.String())
	}
}
