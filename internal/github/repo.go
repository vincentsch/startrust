package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Repo is the part of GitHub's repository summary that StarTrust uses.
type Repo struct {
	ID          int64     `json:"id"`
	FullName    string    `json:"full_name"`
	HTMLURL     string    `json:"html_url"`
	Description string    `json:"description"`
	Homepage    string    `json:"homepage"`
	CreatedAt   time.Time `json:"created_at"`
	PushedAt    time.Time `json:"pushed_at"`
	Stars       int       `json:"stargazers_count"`
	Forks       int       `json:"forks_count"`
	Watchers    int       `json:"subscribers_count"`
	Archived    bool      `json:"archived"`
	Fork        bool      `json:"fork"`
	Private     bool      `json:"private"`
	Permissions *struct {
		Admin    bool `json:"admin"`
		Maintain bool `json:"maintain"`
		Push     bool `json:"push"`
	} `json:"permissions"`
}

// CanSeeStargazers reports whether GitHub will list who starred this repo for
// the current token. Since July 2026 that needs push access or more.
func (r Repo) CanSeeStargazers() bool {
	return r.Permissions != nil && (r.Permissions.Admin || r.Permissions.Maintain || r.Permissions.Push)
}

// Repo fetches the repository summary. A renamed repo is followed to its
// current name.
func (c *Client) Repo(ctx context.Context, fullName string) (Repo, error) {
	res, err := c.get(ctx, "/repos/"+fullName)
	if err != nil {
		return Repo{}, err
	}
	var repo Repo
	if err := json.Unmarshal(res.body, &repo); err != nil {
		return Repo{}, badResponse("/repos/"+fullName, err)
	}
	if repo.ID == 0 || repo.FullName == "" {
		return Repo{}, badResponse("/repos/"+fullName, fmt.Errorf("missing id or name"))
	}
	return repo, nil
}

// Week is one row of GitHub's star history: the Sunday that starts the week
// (00:00 UTC, but the days follow US Pacific time) and the stars on each day,
// Sunday first.
type Week struct {
	Week  int64  `json:"week"`
	Total int    `json:"total"`
	Days  [7]int `json:"days"`
}

// HistoryWeeksPerPage is the page size GitHub uses for star history. It
// ignores larger per_page values.
const HistoryWeeksPerPage = 30

// HistoryPages estimates how many requests the star history of a repo
// created at the given time needs.
func HistoryPages(created, now time.Time) int {
	weeks := int(now.Sub(created).Hours()/(24*7)) + 2
	return (weeks + HistoryWeeksPerPage - 1) / HistoryWeeksPerPage
}

// StarHistory reads the daily star counts of a public repo, newest week
// first, following GitHub's page links to the repo's creation. GitHub only
// counts stars that still exist.
//
// onPage is called after each page with the number of pages read. If a later
// page fails, the weeks read so far are returned together with the error, so
// the caller can still use them.
func (c *Client) StarHistory(ctx context.Context, fullName string, onPage func(page int)) ([]Week, error) {
	var weeks []Week
	target := fmt.Sprintf("/repos/%s/stargazers/history?per_page=%d", fullName, HistoryWeeksPerPage)
	seen := map[string]bool{}
	for page := 1; target != ""; page++ {
		if seen[target] || page > 200 {
			return weeks, badResponse(target, fmt.Errorf("page links loop"))
		}
		seen[target] = true
		res, err := c.get(ctx, target)
		if err != nil {
			return weeks, err
		}
		var batch []Week
		if err := json.Unmarshal(res.body, &batch); err != nil {
			return weeks, badResponse(pathOf(target), err)
		}
		for _, week := range batch {
			if err := week.check(); err != nil {
				return weeks, badResponse(pathOf(target), err)
			}
		}
		weeks = append(weeks, batch...)
		if onPage != nil {
			onPage(page)
		}
		target = nextLink(res.header.Get("Link"))
	}
	return weeks, nil
}

func (w Week) check() error {
	if w.Week <= 0 || w.Week%86400 != 0 {
		return fmt.Errorf("week %d does not start at midnight", w.Week)
	}
	sum := 0
	for _, n := range w.Days {
		if n < 0 {
			return fmt.Errorf("negative star count in week %d", w.Week)
		}
		sum += n
	}
	if sum != w.Total {
		return fmt.Errorf("week %d: days add up to %d, total says %d", w.Week, sum, w.Total)
	}
	return nil
}

// Release is a published GitHub release.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
}

// Releases lists up to 300 of the repo's most recent releases.
func (c *Client) Releases(ctx context.Context, fullName string) ([]Release, error) {
	var all []Release
	target := "/repos/" + fullName + "/releases?per_page=100"
	for page := 1; target != "" && page <= 3; page++ {
		res, err := c.get(ctx, target)
		if err != nil {
			return all, err
		}
		var batch []Release
		if err := json.Unmarshal(res.body, &batch); err != nil {
			return all, badResponse(pathOf(target), err)
		}
		for _, release := range batch {
			if !release.Draft && !release.PublishedAt.IsZero() {
				all = append(all, release)
			}
		}
		target = nextLink(res.header.Get("Link"))
	}
	return all, nil
}

// IssuesAndPullRequestsSince counts the issues and pull requests opened in
// a repo on or after the given day, with one search request. GitHub allows
// 10 searches a minute without a token and 30 with one, apart from the
// hourly limit.
func (c *Client) IssuesAndPullRequestsSince(ctx context.Context, fullName string, since time.Time) (int, error) {
	query := url.QueryEscape(fmt.Sprintf("repo:%s created:>=%s", fullName, since.Format("2006-01-02")))
	res, err := c.get(ctx, "/search/issues?per_page=1&q="+query)
	if err != nil {
		return 0, err
	}
	var found struct {
		TotalCount *int `json:"total_count"`
		Incomplete bool `json:"incomplete_results"`
	}
	if err := json.Unmarshal(res.body, &found); err != nil {
		return 0, badResponse("/search/issues", err)
	}
	switch {
	case found.TotalCount == nil:
		return 0, badResponse("/search/issues", fmt.Errorf("missing total_count"))
	case found.Incomplete:
		// GitHub stops a slow search early and says so. The count would be
		// too low.
		return 0, badResponse("/search/issues", fmt.Errorf("the search timed out"))
	}
	return *found.TotalCount, nil
}

func badResponse(path string, err error) *Error {
	return &Error{Kind: ErrUpstream, Status: 200, Path: path, Err: fmt.Errorf("unexpected response: %w", err)}
}

// ValidName reports whether s looks like owner/name.
func ValidName(s string) bool {
	owner, name, ok := strings.Cut(s, "/")
	return ok && validPart(owner) && validPart(name) && !strings.Contains(name, "/")
}

func validPart(s string) bool {
	if s == "" || len(s) > 100 || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		isLetter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		isDigit := r >= '0' && r <= '9'
		if !isLetter && !isDigit && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}

// ParseName accepts owner/name, a github.com URL or a git remote and returns
// owner/name.
func ParseName(input string) (string, bool) {
	s := strings.TrimSpace(input)
	if strings.Contains(strings.ToLower(s), "github.com") {
		// A URL copied from the browser can end in ?tab=readme-ov-file
		// or #readme.
		if i := strings.IndexAny(s, "?#"); i >= 0 {
			s = s[:i]
		}
	}
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	for _, prefix := range []string{
		"https://github.com/", "http://github.com/", "https://www.github.com/", "http://www.github.com/",
		"www.github.com/", "github.com/", "git@github.com:", "ssh://git@github.com/",
	} {
		if strings.HasPrefix(strings.ToLower(s), prefix) {
			s = s[len(prefix):]
			break
		}
	}
	// Drop extra path parts such as /tree/main from a pasted URL.
	if parts := strings.Split(s, "/"); len(parts) > 2 && strings.Contains(input, "github.com") {
		s = parts[0] + "/" + parts[1]
	}
	return s, ValidName(s)
}
