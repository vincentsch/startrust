// Package check runs a star check on one repo and decides the verdict.
package check

import (
	"time"

	"github.com/vincentsch/startrust/internal/archive"
	"github.com/vincentsch/startrust/internal/stars"
)

// SchemaVersion changes when the JSON layout of Result changes in a way
// that breaks readers.
const SchemaVersion = 1

// Risk levels, from least to most worrying. Unknown means there was not
// enough data to say anything.
const (
	RiskUnknown = "unknown"
	RiskLow     = "low"
	RiskMedium  = "medium"
	RiskHigh    = "high"
)

// RiskRank orders risk levels for --fail-on: unknown 0, low 1, medium 2,
// high 3. It returns -1 for anything else.
func RiskRank(risk string) int {
	switch risk {
	case RiskUnknown:
		return 0
	case RiskLow:
		return 1
	case RiskMedium:
		return 2
	case RiskHigh:
		return 3
	}
	return -1
}

// Result is a finished check. Its JSON form is what `--json` prints and what
// the cache stores.
type Result struct {
	SchemaVersion int    `json:"schema_version"`
	Repository    string `json:"repository"`
	URL           string `json:"url"`
	// FormerNames are earlier names of the repo that GitHub forwards to it:
	// the name you checked, when it was an old one, and every --former-name.
	// GH Archive files stars under the name a repo had at the time, so the
	// account check looks up these names too.
	FormerNames []string  `json:"former_names"`
	CheckedAt   time.Time `json:"checked_at"`
	CreatedAt   time.Time `json:"created_at"`
	Stars       int       `json:"stars"`
	Forks       int       `json:"forks"`
	Watchers    int       `json:"watchers"`
	// IssuesAndPRs counts the issues and pull requests opened in the last
	// ActivityDays days. It is nil when they were not counted.
	IssuesAndPRs *int `json:"issues_and_prs_90_days"`
	Archived     bool `json:"archived"`

	Risk    string `json:"risk"`
	Verdict string `json:"verdict"`
	// Headline sums up the verdict in a few words, such as "no unusual
	// pattern" or "too new to judge". The badge shows it.
	Headline string   `json:"headline"`
	Findings []string `json:"findings"`
	Limits   []string `json:"limits"`
	// AccountsChecked says whether the accounts behind the busy days were
	// checked: "all", "some" or "none". With "none", the verdict rests on
	// the timing of the stars.
	AccountsChecked string `json:"accounts_checked"`
	// RenameSuspected is true when GH Archive has no stars at all for the
	// repo, under any of its names, on a busy day from before June 2026.
	// The repo may have had another name then, and a limit says what to
	// run.
	RenameSuspected bool         `json:"rename_suspected"`
	BiggestDay      *Day         `json:"biggest_day"`
	UnusualSpikes   int          `json:"unusual_spikes"`
	Days            []Day        `json:"days"`
	Burst           *stars.Burst `json:"burst"`
	HistoryFrom     time.Time    `json:"history_from"`
	HistoryTo       time.Time    `json:"history_to"`
	HistoryPartial  bool         `json:"history_partial"`

	Sources Sources `json:"sources"`

	// Details holds the full evidence. It is always saved in the cache and
	// printed with --details.
	Details *Details `json:"details,omitempty"`
}

// Day is one of the repo's biggest days, with what is known about it.
type Day struct {
	Day       time.Time `json:"day"`
	Stars     int       `json:"stars"`
	NormalDay float64   `json:"normal_day"`
	// Ratio is the day's stars divided by the normal day, rounded to one
	// decimal. It is nil when the normal day had no stars.
	Ratio *float64 `json:"times_normal"`
	// Spike says which kind of spike the day belongs to: "launch",
	// "unusual", "spike", or "" when it is not part of a spike.
	Spike string `json:"spike"`

	// Judged is true when the day's share of one-off accounts counts. It
	// is false when the accounts were not checked, and then Accounts is
	// nil. Every day has both fields, so a reader never has to guess what
	// a missing one means.
	Judged       bool            `json:"judged"`
	Accounts     *archive.Cohort `json:"accounts"`
	Profiles     *Profiles       `json:"profiles,omitempty"`
	Explanations []Explanation   `json:"explanations,omitempty"`
}

// Profiles summarizes stargazer profiles from GitHub itself. Only
// maintainers can list who starred their repo, so this needs --deep. An
// account can be both new and empty, so New and Empty can overlap.
type Profiles struct {
	Accounts int `json:"accounts"`
	// New accounts were created at most NewAccountDays days before they starred.
	New int `json:"new"`
	// Empty accounts have no followers and no public repositories.
	Empty int `json:"empty"`
	// NewOrEmpty counts the accounts that are new, empty or both.
	NewOrEmpty int `json:"new_or_empty"`
}

// Explanation is public context near a busy day, such as a Hacker News post
// or a release.
type Explanation struct {
	Kind   string    `json:"kind"` // "hacker_news" or "release"
	Day    time.Time `json:"day"`
	Title  string    `json:"title"`
	URL    string    `json:"url,omitempty"`
	Points int       `json:"points,omitempty"`
}

// Sources says where the data came from and what was missing.
type Sources struct {
	History    string `json:"history"`    // "complete" or "partial"
	Archive    string `json:"archive"`    // "used", "partial", "unavailable", "not_needed" or "off"
	Stargazers string `json:"stargazers"` // "listed", "partial", "hidden" or "off"
	Explain    string `json:"explain"`    // "used", "partial" or "off"

	GitHubRequests int `json:"github_requests"`
	ArchiveQueries int `json:"archive_queries"`
	// TokenSource says which GitHub token the check used: "GITHUB_TOKEN",
	// "GH_TOKEN", "gh" (the GitHub CLI login) or "none".
	TokenSource     string `json:"token_source"`
	ArchiveProblem  string `json:"archive_problem,omitempty"`
	ExplainProblem  string `json:"explain_problem,omitempty"`
	HistoryProblem  string `json:"history_problem,omitempty"`
	StargazerReason string `json:"stargazer_problem,omitempty"`
	TokenProblem    string `json:"token_problem,omitempty"`
	// IssuesProblem says why the issue search did not answer. It stays
	// empty after Ctrl-C, when the search never ran.
	IssuesProblem string `json:"issues_problem,omitempty"`
}

// Details is the full evidence behind a result.
type Details struct {
	History   stars.Series  `json:"history"`
	Spikes    []stars.Spike `json:"spikes"`
	NormalDay float64       `json:"normal_day"`
	// RecentDaily is the average stars per day over the last 30 days.
	RecentDaily float64   `json:"recent_daily"`
	Profiles    *Profiles `json:"profiles,omitempty"`
	Stargazers  int       `json:"stargazers_listed,omitempty"`
}

// Summary returns a copy without the full evidence.
func (r Result) Summary() Result {
	r.Details = nil
	return r
}
