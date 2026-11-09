// Package archive asks GH Archive who starred a repo on a given day and how
// active those accounts were around that day.
//
// GH Archive (https://www.gharchive.org) is a public, third-party record of
// GitHub's public events. StarTrust reads it through ClickHouse's free public
// copy (https://play.clickhouse.com). Nothing here needs a GitHub token, and
// no GitHub requests are made.
package archive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DefaultEndpoint is ClickHouse's public read-only playground.
const DefaultEndpoint = "https://play.clickhouse.com/?user=play"

// The rules for a one-off account and for a usable sample. See
// docs/how-it-works.md for how they were chosen.
const (
	// An account is one-off when the star on this repo is the only star it
	// gave in the WindowDays days before and after. People who star a repo
	// because they found it usually also star other repos. Accounts used in
	// star campaigns mostly do not.
	WindowDays = 30

	// SampleSize caps how many accounts are checked per day. Larger days
	// use a fixed random sample of this size.
	SampleSize = 1000

	// A day can only be judged when GH Archive has at least MinAccounts of
	// its stargazers and at least MinCoverage of the stars GitHub counts.
	// The same share must hold for the days around it. When GH Archive
	// misses stars there, the accounts' other stars also go missing, and
	// they look one-off.
	MinAccounts = 30
	MinCoverage = 0.4
)

// Cohort describes the accounts that starred a repo on one day.
type Cohort struct {
	Day time.Time `json:"day"`
	// GitHubStars is the stars GitHub counts for that day.
	GitHubStars int `json:"github_stars"`
	// Archived is how many accounts GH Archive saw starring that day.
	Archived int `json:"archived"`
	// WindowRecorded compares the stars GH Archive recorded across all of
	// GitHub per day in the WindowDays before and after the day with the
	// week around the day. Under 1 means GH Archive missed more stars
	// around the day than on it.
	WindowRecorded float64 `json:"window_recorded"`
	// Checked is how many of them were checked (at most SampleSize). It
	// stays 0 on a day that cannot be judged.
	Checked int `json:"checked"`
	// OneOff is how many checked accounts starred no other repo in the
	// window around the day.
	OneOff int `json:"one_off"`
	// Problem says why the day could not be judged; empty when it could.
	Problem string `json:"problem,omitempty"`
	// Provisional is set while the WindowDays after the day are not over.
	// Until then, an account that stars another repo next week still
	// looks one-off, so the share does not count yet.
	Provisional bool `json:"provisional,omitempty"`
}

// SettledOn returns the last day before a day's account check counts: the
// window after the day plus two days for GH Archive to catch up.
func SettledOn(day time.Time) time.Time { return day.AddDate(0, 0, WindowDays+2) }

// Settled reports whether a day's account check counts on today.
func Settled(day, today time.Time) bool { return today.After(SettledOn(day)) }

// Usable reports whether the day had enough archive data to judge.
func (c Cohort) Usable() bool { return c.Problem == "" }

// Judged reports whether the share of one-off accounts counts: the day had
// enough archive data, and its window is over. Only then is OneOff out of
// Checked worth reading.
func (c Cohort) Judged() bool { return c.Usable() && !c.Provisional }

// Sampled reports whether GH Archive had more accounts than were checked,
// so the share comes from a sample of SampleSize of them.
func (c Cohort) Sampled() bool { return c.Checked > 0 && c.Checked < c.Archived }

// Share returns the one-off share of the checked accounts.
func (c Cohort) Share() float64 {
	if c.Checked == 0 {
		return 0
	}
	return float64(c.OneOff) / float64(c.Checked)
}

// dayCoverage is the share of the day's stars that GH Archive has, at
// most 1. GH Archive can hold more than GitHub counts, because GitHub drops
// the stars of deleted accounts and GH Archive still has them.
func (c Cohort) dayCoverage() float64 {
	if c.GitHubStars <= 0 {
		return 1
	}
	return min(1, float64(c.Archived)/float64(c.GitHubStars))
}

// Judge decides whether the day can be judged on today. A day whose window
// is not over yet is provisional. A day without enough archive data gets a
// Problem and loses its one-off counts, so nobody reads a share into them.
func (c *Cohort) Judge(today time.Time) {
	c.Provisional = !Settled(c.Day, today)
	c.Problem = ""
	// The days after a provisional day have not all happened, so only the
	// day itself can be measured.
	window := 1.0
	if !c.Provisional {
		window = min(1, c.WindowRecorded)
	}
	switch {
	case c.Archived < MinAccounts || c.dayCoverage() < MinCoverage:
		c.Problem = fmt.Sprintf("GH Archive has only %s of the %s stars from this day", thousands(c.Archived), thousands(c.GitHubStars))
	case c.dayCoverage()*window < MinCoverage:
		c.Problem = fmt.Sprintf("GH Archive recorded only %.0f%% as many stars across GitHub in the %d days before and after this day as in its week",
			100*c.WindowRecorded, WindowDays)
	}
	if c.Problem != "" {
		c.Checked, c.OneOff = 0, 0
	}
}

// thousands formats a count with commas, like the rest of the output.
func thousands(n int) string {
	digits := strconv.Itoa(n)
	for i := len(digits) - 3; i > 0 && digits[i-1] != '-'; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	return digits
}

// Gap reports whether the day itself was recorded well enough but the days
// around it were not.
func (c Cohort) Gap() bool {
	return c.Problem != "" && c.Archived >= MinAccounts && c.dayCoverage() >= MinCoverage
}

// Client runs cohort queries.
type Client struct {
	Endpoint string
	HTTP     *http.Client
	queries  int
}

// Queries returns how many queries the client has sent.
func (c *Client) Queries() int { return c.queries }

// ErrUnavailable wraps every failure to reach or use the archive. Callers
// treat it as "no account data" and carry on.
var ErrUnavailable = errors.New("GH Archive lookup unavailable")

// stoppedEarly is the reason given when the check was interrupted.
const stoppedEarly = "the check was stopped (Ctrl-C) before GH Archive answered"

var safeName = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// Cohort checks the accounts that starred any of the given repo names on
// day (a GitHub calendar day, US Pacific time) and judges the answer on
// today. Pass the current name and, for a renamed repo, the name you asked
// for: GH Archive records events under the name the repo had at the time.
func (c *Client) Cohort(ctx context.Context, names []string, day time.Time, githubStars int, today time.Time) (Cohort, error) {
	result := Cohort{Day: day, GitHubStars: githubStars}
	var quoted []string
	for _, name := range names {
		if !safeName.MatchString(name) {
			return result, fmt.Errorf("%w: invalid repository name %q", ErrUnavailable, name)
		}
		quoted = append(quoted, "'"+name+"'")
	}
	query := fmt.Sprintf(cohortSQL, day.Format("2006-01-02"), strings.Join(quoted, ", "), SampleSize, WindowDays)

	var rows [][]json.Number
	if err := c.run(ctx, query, &rows); err != nil {
		return result, err
	}
	if len(rows) != 1 || len(rows[0]) != 5 {
		return result, fmt.Errorf("%w: unexpected answer", ErrUnavailable)
	}
	values := make([]int, 5)
	for i, v := range rows[0] {
		n, err := v.Int64()
		if err != nil {
			return result, fmt.Errorf("%w: unexpected answer", ErrUnavailable)
		}
		values[i] = int(n)
	}
	result.Archived, result.Checked, result.OneOff = values[0], values[1], values[2]
	result.WindowRecorded = windowRecorded(values[3], values[4])
	result.Judge(today)
	return result, nil
}

// windowRecorded compares the stars per day in the whole window with the
// stars per day in the week around the day, rounded to two decimals.
func windowRecorded(windowStars, weekStars int) float64 {
	if weekStars == 0 {
		return 0
	}
	perDay := float64(windowStars) / float64(2*WindowDays+1)
	return math.Round(100*perDay/(float64(weekStars)/7)) / 100
}

// cohortSQL finds the accounts that starred on the day (on GitHub's Pacific
// calendar), takes a fixed sample, and counts the stars each account gave in
// the window around the day. It also counts every star GH Archive recorded
// in the window and in the week around the day, to see whether the window
// has gaps. The query only counts star events. The table is sorted by event
// type, so ClickHouse reads about half a billion rows instead of all eleven
// billion.
const cohortSQL = `
WITH toDate('%[1]s') AS day,
(
    SELECT (count(), countIf(created_at >= day - 3 AND created_at < day + 4))
    FROM github_events
    WHERE event_type = 'WatchEvent'
      AND created_at >= day - %[4]d AND created_at < day + %[4]d + 1
) AS volume,
stargazers AS (
    SELECT DISTINCT actor_login FROM github_events
    WHERE event_type = 'WatchEvent' AND repo_name IN (%[2]s)
      AND created_at >= day - 1 AND created_at < day + 2
      AND toDate(created_at, 'America/Los_Angeles') = day
),
sample AS (
    SELECT actor_login FROM stargazers ORDER BY cityHash64(actor_login) LIMIT %[3]d
)
SELECT
    (SELECT count() FROM stargazers) AS archived,
    count() AS checked,
    countIf(stars <= 1) AS one_off,
    volume.1 AS window_stars,
    volume.2 AS week_stars
FROM (
    SELECT actor_login, count() AS stars FROM github_events
    WHERE event_type = 'WatchEvent'
      AND actor_login IN (SELECT actor_login FROM sample)
      AND created_at >= day - %[4]d AND created_at < day + %[4]d + 1
    GROUP BY actor_login
)`

func (c *Client) run(parent context.Context, query string, dst any) error {
	if parent.Err() != nil {
		return fmt.Errorf("%w: %s", ErrUnavailable, stoppedEarly)
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	params := u.Query()
	params.Set("default_format", "JSONCompact")
	params.Set("readonly", "1")
	u.RawQuery = params.Encode()

	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(query))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("User-Agent", "startrust")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	c.queries++
	res, err := client.Do(req)
	if err != nil {
		switch {
		case parent.Err() != nil:
			return fmt.Errorf("%w: %s", ErrUnavailable, stoppedEarly)
		case ctx.Err() != nil:
			return fmt.Errorf("%w: the query took longer than 60 seconds", ErrUnavailable)
		}
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s", ErrUnavailable, explain(res.StatusCode, string(body)))
	}
	var answer struct {
		Data json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&answer); err != nil {
		return fmt.Errorf("%w: unexpected answer", ErrUnavailable)
	}
	decoder = json.NewDecoder(strings.NewReader(string(answer.Data)))
	decoder.UseNumber()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("%w: unexpected answer", ErrUnavailable)
	}
	return nil
}

// explain turns a ClickHouse error body into a short reason.
func explain(status int, body string) string {
	switch {
	case strings.Contains(body, "QUOTA_EXCEEDED"):
		return "the free public endpoint's hourly limit is used up; try again within the hour"
	case strings.Contains(body, "TIMEOUT_EXCEEDED"):
		return "the query took too long"
	case status == http.StatusServiceUnavailable || status == http.StatusBadGateway:
		return fmt.Sprintf("the endpoint is down (HTTP %d)", status)
	}
	first, _, _ := strings.Cut(strings.TrimSpace(body), "\n")
	if len(first) > 160 {
		first = first[:160] + "..."
	}
	return fmt.Sprintf("HTTP %d: %s", status, first)
}
