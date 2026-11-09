package check

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vincentsch/startrust/internal/archive"
	"github.com/vincentsch/startrust/internal/cache"
	"github.com/vincentsch/startrust/internal/explain"
	"github.com/vincentsch/startrust/internal/github"
	"github.com/vincentsch/startrust/internal/stars"
)

// ArchiveDays is how many busy days get an account check per repo.
const ArchiveDays = 3

// ArchiveMinStars is the smallest day worth an account check.
const ArchiveMinStars = 50

// ActivityDays is how far back a check counts new issues and pull requests.
const ActivityDays = 90

// Progress receives status lines while a check runs.
type Progress interface {
	// Status replaces the current status line.
	Status(message string)
	// Note prints a line that stays on screen.
	Note(message string)
}

type silent struct{}

func (silent) Status(string) {}
func (silent) Note(string)   {}

// Runner checks repos. It needs GitHub; leave Archive or HN nil to leave
// those lookups out.
type Runner struct {
	GitHub *github.Client
	// TokenSource says where GitHub.Token came from: "GITHUB_TOKEN",
	// "GH_TOKEN" or "gh".
	TokenSource string
	Archive     *archive.Client
	HN          *explain.HN
	Cache       *cache.Store
	Deep        bool
	Progress    Progress
	Now         func() time.Time
}

func (r *Runner) progress() Progress {
	if r.Progress == nil {
		return silent{}
	}
	return r.Progress
}

func (r *Runner) now() time.Time {
	if r.Now == nil {
		return time.Now()
	}
	return r.Now()
}

// Run checks one repo. It returns an error only when nothing useful could be
// read; partial data gives a result with the gaps listed in its limits.
//
// formerNames are earlier names of the repo (--former-name). GitHub must
// forward each of them to the repo, or Run returns a FormerNameError.
func (r *Runner) Run(ctx context.Context, name string, formerNames ...string) (*Result, error) {
	progress := r.progress()
	// A Runner can check several repos in a row (compare), so count this
	// check's requests from here.
	githubBefore, archiveBefore := r.GitHub.Requests(), r.archiveQueries()

	progress.Status("Looking up " + name + " on GitHub")
	repo, err := r.GitHub.Repo(ctx, name)
	tokenRejected := false
	if github.IsKind(err, github.ErrAuth) && r.GitHub.Token != "" {
		// An expired token should not stop a check of public data.
		tokenRejected = true
		r.GitHub.Token = ""
		progress.Note(fmt.Sprintf("GitHub rejected %s, so this check runs without a token (60 requests an hour).", tokenName(r.TokenSource)))
		repo, err = r.GitHub.Repo(ctx, name)
	}
	var former []string
	if err == nil {
		former, err = r.formerNames(ctx, repo, name, formerNames)
	}
	if err != nil {
		if tokenRejected && github.IsKind(err, github.ErrRateLimited) {
			return nil, tokenAndLimitError{err, r.TokenSource}
		}
		return nil, err
	}
	now := r.now()
	today := stars.Today(now)
	r.announce(repo, now)

	pages := github.HistoryPages(repo.CreatedAt, now)
	weeks, historyErr := r.GitHub.StarHistory(ctx, repo.FullName, func(page int) {
		progress.Status(fmt.Sprintf("Reading star history: page %d of about %d", page, pages))
	})
	// After Ctrl-C or an error halfway, the weeks read so far still make a
	// report; it says that it is partial.
	if historyErr != nil && len(weeks) == 0 {
		return nil, fmt.Errorf("cannot read the star history of %s: %w", repo.FullName, historyErr)
	}
	series := stars.FromWeeks(weeks, repo.CreatedAt, today)
	analysis := stars.Analyze(series)

	result := newResult(repo, series, analysis, now)
	result.FormerNames = former
	result.Sources.TokenSource = "none"
	if r.GitHub.Token != "" {
		result.Sources.TokenSource = r.TokenSource
	}
	if tokenRejected {
		result.Sources.TokenProblem = fmt.Sprintf("GitHub rejected %s (HTTP 401), so this check ran without a token. %s",
			tokenName(r.TokenSource), tokenAdvice(r.TokenSource))
	}
	if historyErr != nil {
		result.HistoryPartial = true
		result.Sources.History = "partial"
		result.Sources.HistoryProblem = historyErr.Error()
	}

	r.countIssues(ctx, result, repo, today)
	if r.Archive != nil {
		r.checkAccounts(ctx, result, repo, today)
	}
	if r.Deep {
		r.listStargazers(ctx, result, repo)
	}
	if r.HN != nil {
		r.explain(ctx, result, repo)
	}

	// The biggest day is usually one of the listed days, which now carry
	// their account checks.
	if analysis.Total > 0 {
		biggest, listed := dayAt(result.Days, analysis.BiggestDay)
		if !listed {
			biggest = Day{Day: analysis.BiggestDay, Stars: analysis.BiggestDayStars, NormalDay: analysis.NormalDay,
				Ratio: timesNormal(analysis.BiggestDayStars, analysis.NormalDay)}
		}
		result.BiggestDay = &biggest
	}
	Decide(result, analysis)
	result.Sources.GitHubRequests = r.GitHub.Requests() - githubBefore
	if rate, ok := r.GitHub.RateLimit(); ok {
		result.Sources.GitHubRateLimit = &rate
	}
	result.Sources.ArchiveQueries = r.archiveQueries() - archiveBefore
	progress.Status("")

	if ctx.Err() != nil {
		// An interrupted check is not saved: report and badge must never
		// show half a check as the latest one.
		result.Limits = append(result.Limits, "The check was stopped early (Ctrl-C), so parts of it are missing.")
		return result, nil
	}
	if r.Cache != nil {
		if data, err := json.Marshal(result); err == nil {
			_, _ = r.Cache.SaveCheck(result.Repository, result.CheckedAt, data)
			// A check under a renamed repo's old name is also saved under
			// that name, so `report old/name` finds it right after.
			if !strings.EqualFold(name, result.Repository) {
				_, _ = r.Cache.SaveCheck(name, result.CheckedAt, data)
			}
		}
		// Leave the in-progress file in place when the stargazer list is
		// unfinished, so the next run continues instead of starting over.
		if result.Sources.Stargazers != "partial" {
			r.Cache.ClearProgress(result.Repository)
		}
	}
	return result, nil
}

// tokenAndLimitError explains a rate limit that hit after GitHub rejected
// the token: the usual advice to set a token would be wrong, because the
// user did.
type tokenAndLimitError struct {
	err    error
	source string
}

func (e tokenAndLimitError) Error() string {
	msg := fmt.Sprintf("GitHub rejected %s (HTTP 401), and without a token its limit of 60 requests an hour is used up", tokenName(e.source))
	var apiErr *github.Error
	if errors.As(e.err, &apiErr) && !apiErr.ResetAt.IsZero() {
		msg += "; it resets at " + apiErr.ResetAt.Local().Format("15:04")
	}
	return msg + ". " + tokenAdvice(e.source)
}

// tokenName says where a token came from, so a message about it points at
// the right place.
func tokenName(source string) string {
	switch source {
	case "GITHUB_TOKEN", "GH_TOKEN":
		return "the token in " + source
	case "gh":
		return "the token of your GitHub CLI login"
	}
	return "the token"
}

// tokenAdvice says how to replace a token GitHub rejected.
func tokenAdvice(source string) string {
	switch source {
	case "GITHUB_TOKEN", "GH_TOKEN":
		return "Set a new token in " + source
	case "gh":
		return "Run `gh auth login` to refresh it"
	}
	return "Refresh it with `gh auth login` or a new GITHUB_TOKEN"
}

func (e tokenAndLimitError) Unwrap() error { return e.err }

// FormerNameError means that GitHub does not forward a --former-name to the
// repo, so StarTrust cannot tell that the name was ever the repo's own.
type FormerNameError struct {
	Name string // the --former-name
	Repo string // the repo's current name
	// Other is the repo GitHub has under that name now, or "" when it has
	// none.
	Other string
}

func (e FormerNameError) Error() string {
	rule := "StarTrust only uses a former name that GitHub still forwards to " + e.Repo
	switch {
	case e.Other == "":
		return fmt.Sprintf("--former-name %s: GitHub has no repo by that name. %s, so check the spelling", e.Name, rule)
	case strings.EqualFold(e.Other, e.Name):
		return fmt.Sprintf("--former-name %s: GitHub has a different repo under that name. %s, because GH Archive would add the other repo's stars",
			e.Name, rule)
	}
	return fmt.Sprintf("--former-name %s: GitHub forwards it to %s, a different repo. %s, because GH Archive would add the other repo's stars",
		e.Name, e.Other, rule)
}

// formerNames returns the earlier names of the repo to look up in GH
// Archive: the name the user checked, when GitHub answered with the repo
// under another name, and every --former-name. GitHub forwards the old
// names of a renamed repo to it, so a --former-name that leads nowhere or to
// another repo is refused.
func (r *Runner) formerNames(ctx context.Context, repo github.Repo, asked string, given []string) ([]string, error) {
	names := []string{}
	known := func(name string) bool {
		if strings.EqualFold(name, repo.FullName) {
			return true
		}
		for _, n := range names {
			if strings.EqualFold(n, name) {
				return true
			}
		}
		return false
	}
	if !known(asked) {
		names = append(names, asked)
	}
	for _, name := range given {
		if known(name) {
			continue
		}
		r.progress().Status("Asking GitHub whether " + name + " leads to " + repo.FullName)
		other, err := r.GitHub.Repo(ctx, name)
		switch {
		case github.IsKind(err, github.ErrNotFound):
			return nil, FormerNameError{Name: name, Repo: repo.FullName}
		case err != nil:
			return nil, err
		case other.ID != repo.ID:
			return nil, FormerNameError{Name: name, Repo: repo.FullName, Other: other.FullName}
		}
		names = append(names, name)
	}
	return names, nil
}

// announce prints what the check is about to do and what it will cost.
func (r *Runner) announce(repo github.Repo, now time.Time) {
	pages := github.HistoryPages(repo.CreatedAt, now)
	// The repo summary, the history pages and one search.
	estimate := fmt.Sprintf("%s: %s stars since %s. About %s", repo.FullName, Number(repo.Stars), date(repo.CreatedAt),
		plural(pages+2, "GitHub request", "GitHub requests"))
	if r.Deep && repo.CanSeeStargazers() {
		estimate += fmt.Sprintf(" plus %s for the stargazer list", plural((repo.Stars+99)/100, "request", "requests"))
	}
	if r.Archive != nil {
		estimate += fmt.Sprintf(" and up to %d GH Archive queries", BigDays)
	}
	r.progress().Note(estimate + ".")
	if rate, ok := r.GitHub.RateLimit(); ok && rate.Remaining < pages {
		r.progress().Note(fmt.Sprintf("GitHub allows %d more requests until %s. The check will use what it can get.",
			rate.Remaining, rate.ResetsAt.Local().Format("15:04")))
	}
}

func (r *Runner) archiveQueries() int {
	if r.Archive == nil {
		return 0
	}
	return r.Archive.Queries()
}

// newResult fills in what the repo summary and the star history say.
func newResult(repo github.Repo, series stars.Series, analysis stars.Analysis, now time.Time) *Result {
	result := &Result{
		SchemaVersion: SchemaVersion,
		Repository:    repo.FullName,
		URL:           repo.HTMLURL,
		CheckedAt:     now.UTC().Truncate(time.Second),
		CreatedAt:     repo.CreatedAt,
		Stars:         repo.Stars,
		Forks:         repo.Forks,
		Watchers:      repo.Watchers,
		Archived:      repo.Archived,
		Days:          BiggestDays(series, analysis, BigDays),
		Sources:       Sources{History: "complete", Archive: "off", Stargazers: "off", Explain: "off"},
		Details: &Details{
			History:     series,
			Spikes:      analysis.Spikes,
			NormalDay:   analysis.NormalDay,
			RecentDaily: analysis.RecentDaily,
		},
	}
	if len(series.Daily) > 0 {
		result.HistoryFrom, result.HistoryTo = series.Start, series.End()
	}
	return result
}

// countIssues counts the issues and pull requests opened in the last
// ActivityDays days. They show how many people use the repo, next to how
// many starred it. When the search fails, the count stays unknown and the
// check goes on.
func (r *Runner) countIssues(ctx context.Context, result *Result, repo github.Repo, today time.Time) {
	if ctx.Err() != nil {
		return
	}
	r.progress().Status("Counting recent issues and pull requests")
	n, err := r.GitHub.IssuesAndPullRequestsSince(ctx, repo.FullName, today.AddDate(0, 0, -ActivityDays))
	switch {
	case err == nil:
		result.IssuesAndPRs = &n
	case ctx.Err() != nil:
		// Ctrl-C has its own limit line.
	case github.IsKind(err, github.ErrRateLimited):
		// The usual rate limit message talks about the hourly limit, but
		// search has a limit per minute.
		result.Sources.IssuesProblem = "GitHub allows 10 searches a minute without a token and 30 with one, and that limit was used up"
	default:
		result.Sources.IssuesProblem = err.Error()
	}
}

// checkAccounts asks GH Archive about the busiest days.
func (r *Runner) checkAccounts(ctx context.Context, result *Result, repo github.Repo, today time.Time) {
	targets := AccountTargets(result.Days, result.Details.Spikes)
	if len(targets) == 0 {
		result.Sources.Archive = "not_needed"
		return
	}
	// GH Archive files events under the name a repo had at the time, so a
	// renamed repo is also looked up under its former names. The answer
	// depends on all the names, and so does where it is cached.
	names := append([]string{repo.FullName}, result.FormerNames...)
	progress := r.progress()
	for n, i := range targets {
		day := &result.Days[i]
		key := date(day.Day) + namesKey(result.FormerNames)
		var cohort archive.Cohort
		if r.Cache != nil && r.Cache.Load("accounts", repo.FullName, key, &cohort) && cohort.GitHubStars == day.Stars {
			day.Accounts = &cohort
			continue
		}
		progress.Status(fmt.Sprintf("Checking the accounts behind busy day %d of %d in GH Archive", n+1, len(targets)))
		cohort, err := r.Archive.Cohort(ctx, names, day.Day, day.Stars, today)
		if err != nil {
			result.Sources.ArchiveProblem = strings.TrimPrefix(err.Error(), archive.ErrUnavailable.Error()+": ")
			if ctx.Err() != nil {
				break
			}
			continue
		}
		// Answers for days whose window is not over yet still change, so
		// they are not cached.
		day.Accounts = &cohort
		if r.Cache != nil && !cohort.Provisional {
			_ = r.Cache.Save("accounts", repo.FullName, key, cohort)
		}
	}
	answered := 0
	for _, i := range targets {
		if result.Days[i].Accounts != nil {
			answered++
		}
	}
	switch answered {
	case 0:
		result.Sources.Archive = "unavailable"
	case len(targets):
		result.Sources.Archive = "used"
	default:
		result.Sources.Archive = "partial"
	}
}

// namesKey tells apart the cached answers for the same day under different
// sets of former names. It is empty without former names. GH Archive
// matches names exactly, so the key keeps their spelling.
func namesKey(names []string) string {
	if len(names) == 0 {
		return ""
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, " ")))
	return "-" + hex.EncodeToString(sum[:6])
}

// inProgress is the state saved while stargazers are listed, so an
// interrupted run can continue.
type inProgress struct {
	Repository string             `json:"repository"`
	SavedAt    time.Time          `json:"saved_at"`
	Cursor     string             `json:"cursor"`
	Stargazers []github.Stargazer `json:"stargazers"`
}

// listStargazers reads who starred the repo, when GitHub lets the token see
// that, and compares the profiles behind each busy day.
func (r *Runner) listStargazers(ctx context.Context, result *Result, repo github.Repo) {
	if !repo.CanSeeStargazers() {
		result.Sources.Stargazers = "hidden"
		result.Sources.StargazerReason = "--deep needs push access to the repo, because GitHub shows who starred a repo only to its maintainers"
		return
	}
	var saved inProgress
	cursor := ""
	var earlier []github.Stargazer
	if r.Cache != nil && r.Cache.LoadProgress(repo.FullName, &saved) &&
		strings.EqualFold(saved.Repository, repo.FullName) && r.now().Sub(saved.SavedAt) < 24*time.Hour {
		cursor, earlier = saved.Cursor, saved.Stargazers
		r.progress().Note(fmt.Sprintf("Continuing an earlier run, with %s stargazers already listed.", Number(len(earlier))))
	}
	listed, err := r.GitHub.Stargazers(ctx, repo.FullName, cursor, func(batch []github.Stargazer, next string) {
		all := append(append([]github.Stargazer(nil), earlier...), batch...)
		r.progress().Status(fmt.Sprintf("Listing stargazers: %s of %s", Number(len(all)), Number(repo.Stars)))
		if r.Cache != nil {
			_ = r.Cache.SaveProgress(repo.FullName, inProgress{Repository: repo.FullName, SavedAt: r.now(), Cursor: next, Stargazers: all})
		}
	})
	all := append(earlier, listed...)
	switch {
	case errors.Is(err, github.ErrStargazersHidden):
		result.Sources.Stargazers = "hidden"
		result.Sources.StargazerReason = "GitHub returned no stargazers for this token, so --deep had nothing to compare"
		return
	case err != nil:
		result.Sources.Stargazers = "partial"
		result.Sources.StargazerReason = fmt.Sprintf("Listed %s of %s stargazers before this error: %v. Run the same command again to continue",
			Number(len(all)), Number(repo.Stars), err)
	default:
		result.Sources.Stargazers = "listed"
	}
	overall := profilesOf(all, func(github.Stargazer) bool { return true })
	result.Details.Profiles = &overall
	result.Details.Stargazers = len(all)
	for i := range result.Days {
		day := result.Days[i].Day
		p := profilesOf(all, func(s github.Stargazer) bool { return stars.Today(s.StarredAt).Equal(day) })
		if p.Accounts > 0 {
			result.Days[i].Profiles = &p
		}
	}
}

func profilesOf(list []github.Stargazer, keep func(github.Stargazer) bool) Profiles {
	var p Profiles
	for _, s := range list {
		if !keep(s) {
			continue
		}
		p.Accounts++
		isNew := !s.CreatedAt.IsZero() && s.StarredAt.Sub(s.CreatedAt) <= NewAccountDays*24*time.Hour
		empty := s.Followers == 0 && s.PublicRepos == 0
		if isNew {
			p.New++
		}
		if empty {
			p.Empty++
		}
		if isNew || empty {
			p.NewOrEmpty++
		}
	}
	return p
}

// explain looks for Hacker News posts and releases right before busy days.
func (r *Runner) explain(ctx context.Context, result *Result, repo github.Repo) {
	r.progress().Status("Looking for Hacker News posts and releases near the busy days")
	result.Sources.Explain = "used"
	var problems []string
	posts, err := r.HN.Posts(ctx, repo.FullName, repo.Homepage, 10)
	if err != nil {
		problems = append(problems, err.Error())
	}
	releases, err := r.GitHub.Releases(ctx, repo.FullName)
	if err != nil {
		problems = append(problems, "releases: "+err.Error())
	}
	if len(problems) > 0 {
		result.Sources.Explain = "partial"
		result.Sources.ExplainProblem = strings.Join(problems, "; ")
	}
	for i := range result.Days {
		day := result.Days[i].Day
		near := func(t time.Time) bool {
			d := stars.Today(t)
			return !d.After(day) && day.Sub(d) <= 2*24*time.Hour
		}
		for _, post := range posts {
			if near(post.CreatedAt) {
				result.Days[i].Explanations = append(result.Days[i].Explanations, Explanation{
					Kind: "hacker_news", Day: stars.Today(post.CreatedAt), Title: post.Title, URL: post.Link(), Points: post.Points,
				})
			}
		}
		// Busy projects release every few days, so only the newest release
		// near the day is worth showing.
		var newest *github.Release
		for j := range releases {
			release := &releases[j]
			if near(release.PublishedAt) && !release.Prerelease && (newest == nil || release.PublishedAt.After(newest.PublishedAt)) {
				newest = release
			}
		}
		if newest != nil {
			title := newest.TagName
			if newest.Name != "" && newest.Name != newest.TagName {
				title += " (" + newest.Name + ")"
			}
			result.Days[i].Explanations = append(result.Days[i].Explanations, Explanation{
				Kind: "release", Day: stars.Today(newest.PublishedAt), Title: title,
			})
		}
	}
}
