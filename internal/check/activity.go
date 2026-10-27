package check

import (
	"fmt"
	"math"
)

// Forks, watchers and new issues show how many people use a repo, next to
// how many starred it. StarTrust compared them for 15 flagged and
// OrganicRepos organic repos (docs/how-it-works.md). Most flagged repos had
// normal numbers, and some honest repos have almost no forks, so these
// facts are shown as context and never change the risk.
const (
	// OrganicRepos is how many organic repos the comparison had.
	OrganicRepos = 182
	// OrganicMinForks is the fewest forks per 1,000 stars any organic repo
	// had in that comparison, including the first days after a launch.
	OrganicMinForks = 10
	// ActivityMinStars is the smallest star count for which forks per
	// 1,000 stars are shown. Below it, a few forks more or less swing the
	// number too much.
	ActivityMinStars = 1000
)

// ForksPer1000 returns the repo's forks per 1,000 stars.
func (r Result) ForksPer1000() float64 {
	if r.Stars == 0 {
		return 0
	}
	return 1000 * float64(r.Forks) / float64(r.Stars)
}

// ForksPer1000Text formats ForksPer1000 for people: whole numbers, or one
// decimal under 10, so 0.4 does not read as none.
func (r Result) ForksPer1000Text() string {
	v := r.ForksPer1000()
	if v < 10 && v != math.Round(v) {
		return fmt.Sprintf("%.1f", v)
	}
	return Number(int(math.Round(v)))
}

// IssuesPeriod says which days IssuesAndPRs covers: "so far" for a repo
// younger than ActivityDays, otherwise "in the last 90 days".
func (r Result) IssuesPeriod() string {
	if !r.CreatedAt.IsZero() && r.CheckedAt.Sub(r.CreatedAt).Hours() < ActivityDays*24 {
		return "so far"
	}
	return fmt.Sprintf("in the last %d days", ActivityDays)
}

// ActivityNote tells a reader how to weigh the activity numbers.
func ActivityNote() string {
	return fmt.Sprintf("In StarTrust's comparison of %d organic repos, every one had at least %d forks per 1,000 stars. "+
		"Watchers and issues vary a lot with the kind of repo. But most repos flagged for bought stars "+
		"also had normal numbers here, so normal numbers do not prove that the stars are real. "+
		"Few forks alone also mean nothing, because tools such as `composer thanks` star library repos automatically.",
		OrganicRepos, OrganicMinForks)
}

// activitySentence adds the activity to a verdict that could not judge
// the stars, so the reader still learns how much the repo is used.
func activitySentence(r *Result) string {
	forks := plural(r.Forks, "fork", "forks")
	if r.IssuesAndPRs == nil {
		return fmt.Sprintf("It has %s, see below.", forks)
	}
	issues := plural(*r.IssuesAndPRs, "issue or pull request", "issues and pull requests")
	if period := r.IssuesPeriod(); period != "so far" {
		verb := "were"
		if *r.IssuesAndPRs == 1 {
			verb = "was"
		}
		return fmt.Sprintf("It has %s, and %s %s opened %s, see below.", forks, issues, verb, period)
	}
	return fmt.Sprintf("So far it has %s and %s, see below.", forks, issues)
}
