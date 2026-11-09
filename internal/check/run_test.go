package check

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vincentsch/startrust/internal/archive"
	"github.com/vincentsch/startrust/internal/cache"
	"github.com/vincentsch/startrust/internal/github"
	"github.com/vincentsch/startrust/internal/stars"
)

// ownedRepo fakes a small repo the token can push to, with two pages of
// stargazers. While failSecondPage is set, the second page fails.
func ownedRepo(t *testing.T, failSecondPage *atomic.Bool) *httptest.Server {
	t.Helper()
	sunday := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC).Unix()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.URL.Path == "/repos/me/tool":
			io.WriteString(w, `{"id":7,"full_name":"me/tool","html_url":"https://github.com/me/tool","created_at":"2026-10-18T00:00:00Z",
				"stargazers_count":150,"permissions":{"admin":true,"maintain":true,"push":true}}`)
		case strings.HasSuffix(req.URL.Path, "/stargazers/history"):
			fmt.Fprintf(w, `[{"week":%d,"total":150,"days":[0,140,10,0,0,0,0]}]`, sunday)
		case req.URL.Path == "/graphql":
			var body struct {
				Variables map[string]any `json:"variables"`
			}
			json.NewDecoder(req.Body).Decode(&body)
			page := 1
			if body.Variables["after"] == "cursor-1" {
				page = 2
			}
			if page == 2 && failSecondPage.Load() {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			count := map[int]int{1: 100, 2: 50}[page]
			var edges []string
			for i := 0; i < count; i++ {
				edges = append(edges, fmt.Sprintf(`{"starredAt":"2026-10-19T18:00:00Z","node":{"login":"u%d-%d","createdAt":"2015-01-01T00:00:00Z","followers":{"totalCount":1},"repositories":{"totalCount":1}}}`, page, i))
			}
			fmt.Fprintf(w, `{"data":{"repository":{"stargazerCount":150,"stargazers":{"pageInfo":{"hasNextPage":%v,"endCursor":"cursor-%d"},"edges":[%s]}}}}`,
				page == 1, page, strings.Join(edges, ","))
		default:
			http.NotFound(w, req)
		}
	}))
}

func TestInterruptedStargazerListContinuesNextTime(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	server := ownedRepo(t, &failing)
	defer server.Close()
	store, _ := cache.Open(t.TempDir())
	now := func() time.Time { return time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC) }
	newRunner := func() *Runner {
		return &Runner{
			GitHub: &github.Client{BaseURL: server.URL, Token: "t", Sleep: func(context.Context, time.Duration) error { return nil }},
			Cache:  store, Deep: true, Now: now,
		}
	}

	first, err := newRunner().Run(context.Background(), "me/tool")
	if err != nil {
		t.Fatal(err)
	}
	if first.Sources.Stargazers != "partial" || first.Details.Stargazers != 100 {
		t.Fatalf("first run: %s with %d stargazers", first.Sources.Stargazers, first.Details.Stargazers)
	}
	var saved inProgress
	if !store.LoadProgress("me/tool", &saved) || saved.Cursor != "cursor-1" || len(saved.Stargazers) != 100 {
		t.Fatalf("the in-progress file must survive a partial run: %+v", saved.Cursor)
	}

	failing.Store(false)
	second, err := newRunner().Run(context.Background(), "me/tool")
	if err != nil {
		t.Fatal(err)
	}
	if second.Sources.Stargazers != "listed" || second.Details.Stargazers != 150 {
		t.Fatalf("second run: %s with %d stargazers", second.Sources.Stargazers, second.Details.Stargazers)
	}
	if store.LoadProgress("me/tool", &saved) {
		t.Error("the in-progress file should be gone after a complete list")
	}
}

func TestDeepOnSomeoneElsesRepoIsExplained(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/stargazers/history") {
			io.WriteString(w, `[]`)
			return
		}
		io.WriteString(w, `{"id":1,"full_name":"other/repo","created_at":"2026-01-01T00:00:00Z","stargazers_count":0,"permissions":{"pull":true}}`)
	}))
	defer server.Close()
	r := &Runner{GitHub: &github.Client{BaseURL: server.URL, Token: "t"}, Deep: true}
	result, err := r.Run(context.Background(), "other/repo")
	if err != nil {
		t.Fatal(err)
	}
	if result.Sources.Stargazers != "hidden" || !strings.Contains(strings.Join(result.Limits, " "), "push access") {
		t.Errorf("sources %+v limits %v", result.Sources, result.Limits)
	}
}

func TestNewAndEmptyProfilesAreCountedSeparately(t *testing.T) {
	starred := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	list := []github.Stargazer{
		// New and empty at once: it must count as both.
		{Login: "a", StarredAt: starred, CreatedAt: starred.AddDate(0, 0, -3)},
		// Only empty.
		{Login: "b", StarredAt: starred, CreatedAt: starred.AddDate(-5, 0, 0)},
		// Only new.
		{Login: "c", StarredAt: starred, CreatedAt: starred.AddDate(0, 0, -10), Followers: 2},
		// Neither.
		{Login: "d", StarredAt: starred, CreatedAt: starred.AddDate(-5, 0, 0), PublicRepos: 4},
	}
	got := profilesOf(list, func(github.Stargazer) bool { return true })
	want := Profiles{Accounts: 4, New: 2, Empty: 2, NewOrEmpty: 3}
	if got != want {
		t.Errorf("profiles = %+v, want %+v", got, want)
	}
}

func TestDeepOnASmallRepoSumsUpAllStargazers(t *testing.T) {
	// No day of a 14-star repo has enough stargazers for its own line, but
	// the maintainer asked for --deep, so the findings sum up all of them.
	r := &Result{Stars: 14, CreatedAt: day("2026-08-18"), CheckedAt: day("2026-10-26"),
		Details: &Details{Profiles: &Profiles{Accounts: 14, New: 1, Empty: 2, NewOrEmpty: 3}}}
	daily := make([]int, 70)
	daily[3], daily[42] = 7, 7
	Decide(r, stars.Analyze(stars.Series{Start: day("2026-08-18"), Daily: daily}))
	want := "Of all 14 stargazers, 1 was created in the 30 days before it starred, and 2 have no followers and no public repositories."
	if !strings.Contains(strings.Join(r.Findings, "\n"), want) {
		t.Errorf("findings:\n%s", strings.Join(r.Findings, "\n"))
	}
	if got := overallProfileFinding(Profiles{Accounts: 200, New: 10, Empty: 50}); got !=
		"Of all 200 stargazers, 5% were created in the 30 days before they starred, and 25% have no followers and no public repositories." {
		t.Errorf("big repo: %q", got)
	}
}

func TestYesterdaysLaunchIsProvisional(t *testing.T) {
	// A repo that launched yesterday. GH Archive already has most of the
	// day's stargazers, and 60% of them have no other public stars so far. The
	// 30 days after the launch have not happened, so that share must not
	// count, and the answer must not be cached.
	sunday := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC).Unix()
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/stargazers/history") {
			fmt.Fprintf(w, `[{"week":%d,"total":120,"days":[100,20,0,0,0,0,0]}]`, sunday)
			return
		}
		io.WriteString(w, `{"id":3,"full_name":"me/new","created_at":"2026-10-20T00:00:00Z","stargazers_count":120}`)
	}))
	defer gh.Close()
	clickhouse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, `{"data":[["100","100","60","61000","7000"]]}`)
	}))
	defer clickhouse.Close()
	store, _ := cache.Open(t.TempDir())
	r := &Runner{
		GitHub:  &github.Client{BaseURL: gh.URL},
		Archive: &archive.Client{Endpoint: clickhouse.URL},
		Cache:   store,
		Now:     func() time.Time { return time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC) },
	}
	result, err := r.Run(context.Background(), "me/new")
	if err != nil {
		t.Fatal(err)
	}
	c := result.Days[0].Accounts
	if c == nil || !c.Provisional || c.OneOff != 60 {
		t.Fatalf("accounts = %+v", c)
	}
	if result.Risk != RiskUnknown || !strings.Contains(result.Verdict, "can be checked after 2026-11-26.") {
		t.Errorf("risk %s, verdict %q", result.Risk, result.Verdict)
	}
	var cached archive.Cohort
	if store.Load("accounts", "me/new", "2026-10-25", &cached) {
		t.Error("a provisional answer was cached")
	}
}

func TestOldNameGetsItsOwnAccountCheck(t *testing.T) {
	// GH Archive knows the busy day only under the repo's old name. A check
	// under the new name finds too few accounts; a later check under the
	// old name must ask again instead of reusing that cached answer.
	sunday := time.Date(2023, 4, 9, 0, 0, 0, 0, time.UTC).Unix()
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/stargazers/history") {
			// Three busy days, so three account checks are due.
			fmt.Fprintf(w, `[{"week":%d,"total":500,"days":[0,0,0,0,500,0,0]},{"week":%d,"total":300,"days":[0,0,0,0,300,0,0]},{"week":%d,"total":200,"days":[0,0,0,0,200,0,0]}]`,
				sunday, sunday+7*86400, sunday+14*86400)
			return
		}
		io.WriteString(w, `{"id":4,"full_name":"new/name","created_at":"2023-04-01T00:00:00Z","stargazers_count":500}`)
	}))
	defer gh.Close()
	var queries []string
	clickhouse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		queries = append(queries, string(body))
		if strings.Contains(string(body), "'old/name'") {
			io.WriteString(w, `{"data":[["480","480","10","61000","7000"]]}`)
			return
		}
		io.WriteString(w, `{"data":[["0","0","0","61000","7000"]]}`)
	}))
	defer clickhouse.Close()
	store, _ := cache.Open(t.TempDir())
	newRunner := func() *Runner {
		return &Runner{
			GitHub:  &github.Client{BaseURL: gh.URL},
			Archive: &archive.Client{Endpoint: clickhouse.URL},
			Cache:   store,
			Now:     func() time.Time { return time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC) },
		}
	}
	if _, err := newRunner().Run(context.Background(), "new/name"); err != nil {
		t.Fatal(err)
	}
	result, err := newRunner().Run(context.Background(), "old/name")
	if err != nil {
		t.Fatal(err)
	}
	if c := result.Days[0].Accounts; c == nil || c.Archived != 480 {
		t.Errorf("accounts under the old name = %+v after %d queries", c, len(queries))
	}
	// The result says that GitHub answered for the old name with the repo
	// under its new one.
	if result.Repository != "new/name" || len(result.FormerNames) != 1 || result.FormerNames[0] != "old/name" {
		t.Errorf("repository %s, former names %v", result.Repository, result.FormerNames)
	}
	// `startrust report old/name` must find the check right after.
	if _, _, err := store.LatestCheck("old/name"); err != nil {
		t.Errorf("no saved check under the old name: %v", err)
	}
	if _, _, err := store.LatestCheck("new/name"); err != nil {
		t.Errorf("no saved check under the current name: %v", err)
	}
}

func TestFormerNamesMustLeadToTheRepo(t *testing.T) {
	// GitHub forwards old/name to the repo the way it forwards renamed
	// repos. other/repo is a different repo, and gone/name does not exist.
	// GH Archive knows the busy days only under old/name.
	sunday := time.Date(2023, 4, 9, 0, 0, 0, 0, time.UTC).Unix()
	var history atomic.Int32
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/repos/new/name/stargazers/history":
			history.Add(1)
			fmt.Fprintf(w, `[{"week":%d,"total":500,"days":[0,0,0,0,500,0,0]},{"week":%d,"total":300,"days":[0,0,0,0,300,0,0]}]`,
				sunday, sunday+7*86400)
		case "/repos/new/name", "/repositories/4":
			io.WriteString(w, `{"id":4,"full_name":"new/name","created_at":"2023-04-01T00:00:00Z","stargazers_count":800}`)
		case "/repos/old/name":
			http.Redirect(w, req, "/repositories/4", http.StatusMovedPermanently)
		case "/repos/other/repo":
			io.WriteString(w, `{"id":9,"full_name":"other/repo","created_at":"2020-01-01T00:00:00Z","stargazers_count":10}`)
		default:
			http.NotFound(w, req)
		}
	}))
	defer gh.Close()
	var queries []string
	clickhouse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		queries = append(queries, string(body))
		if strings.Contains(string(body), "'old/name'") {
			io.WriteString(w, `{"data":[["480","480","10","61000","7000"]]}`)
			return
		}
		io.WriteString(w, `{"data":[["0","0","0","61000","7000"]]}`)
	}))
	defer clickhouse.Close()
	store, _ := cache.Open(t.TempDir())
	r := &Runner{
		GitHub:  &github.Client{BaseURL: gh.URL},
		Archive: &archive.Client{Endpoint: clickhouse.URL},
		Cache:   store,
		Now:     func() time.Time { return time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC) },
	}

	// Under its current name alone, GH Archive has nothing, so the check
	// suspects a rename and says what to run.
	result, err := r.Run(context.Background(), "new/name")
	if err != nil {
		t.Fatal(err)
	}
	if !result.RenameSuspected || !strings.Contains(strings.Join(result.Limits, "\n"), "`startrust check new/name --former-name OLD-OWNER/OLD-NAME`") {
		t.Errorf("rename suspected %v, limits:\n%s", result.RenameSuspected, strings.Join(result.Limits, "\n"))
	}

	// With the former name, every query asks for both names, and the
	// answer cached for the current name alone is not reused.
	queries = nil
	result, err = r.Run(context.Background(), "new/name", "old/name", "NEW/NAME")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.FormerNames) != 1 || result.FormerNames[0] != "old/name" || result.RenameSuspected || result.AccountsChecked != "all" {
		t.Errorf("former names %v, rename suspected %v, accounts checked %s", result.FormerNames, result.RenameSuspected, result.AccountsChecked)
	}
	if len(queries) != 2 {
		t.Errorf("%d GH Archive queries, want 2", len(queries))
	}
	for _, q := range queries {
		if !strings.Contains(q, "IN ('new/name', 'old/name')") {
			t.Errorf("query without both names:\n%s", q)
		}
	}

	// A name that leads nowhere, or to another repo, is refused before the
	// check reads anything else.
	before := history.Load()
	for name, want := range map[string]FormerNameError{
		"gone/name":  {Name: "gone/name", Repo: "new/name"},
		"other/repo": {Name: "other/repo", Repo: "new/name", Other: "other/repo"},
	} {
		_, err := r.Run(context.Background(), "new/name", name)
		var got FormerNameError
		if !errors.As(err, &got) || got != want {
			t.Errorf("%s: error %v", name, err)
		}
	}
	if history.Load() != before {
		t.Error("a refused former name still read the star history")
	}
}

func TestCtrlCBeforeTheAccountCheck(t *testing.T) {
	// Ctrl-C right after the history arrived: no GH Archive query runs,
	// the archive counts as unavailable, and nothing is saved.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sunday := time.Date(2023, 4, 9, 0, 0, 0, 0, time.UTC).Unix()
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/stargazers/history") {
			// Three busy days, so three account checks are due.
			fmt.Fprintf(w, `[{"week":%d,"total":500,"days":[0,0,0,0,500,0,0]},{"week":%d,"total":300,"days":[0,0,0,0,300,0,0]},{"week":%d,"total":200,"days":[0,0,0,0,200,0,0]}]`,
				sunday, sunday+7*86400, sunday+14*86400)
			return
		}
		io.WriteString(w, `{"id":4,"full_name":"me/tool","created_at":"2023-04-01T00:00:00Z","stargazers_count":500}`)
	}))
	defer gh.Close()
	queried := false
	clickhouse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		queried = true
	}))
	defer clickhouse.Close()
	store, _ := cache.Open(t.TempDir())
	r := &Runner{
		GitHub:  &github.Client{BaseURL: gh.URL},
		Archive: &archive.Client{Endpoint: clickhouse.URL},
		Cache:   store,
		Now:     func() time.Time { return time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC) },
		// The user presses Ctrl-C as soon as the history page is read.
		Progress: cancelAfter{"Reading star history", cancel},
	}
	result, err := r.Run(ctx, "me/tool")
	if err != nil {
		t.Fatal(err)
	}
	limits := strings.Join(result.Limits, "\n")
	if queried || result.Sources.Archive != "unavailable" || strings.Contains(limits, "Account checks use GH Archive") || !strings.Contains(limits, "stopped early (Ctrl-C)") {
		t.Errorf("queried %v, archive %s, limits:\n%s", queried, result.Sources.Archive, limits)
	}
	if _, _, err := store.LatestCheck("me/tool"); err == nil {
		t.Error("an interrupted check was saved")
	}
}

// cancelAfter cancels a context when a status line starts with prefix.
type cancelAfter struct {
	prefix string
	cancel context.CancelFunc
}

func (c cancelAfter) Status(message string) {
	if strings.HasPrefix(message, c.prefix) {
		c.cancel()
	}
}

func (c cancelAfter) Note(string) {}

func TestCheckCountsRecentIssuesAndPullRequests(t *testing.T) {
	// One search counts the issues and pull requests of the last 90 days.
	// When GitHub's search limit is used up, the count stays unknown and
	// the check still finishes.
	var query string
	var limited atomic.Bool
	sunday := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC).Unix()
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.URL.Path == "/search/issues":
			query = req.URL.Query().Get("q")
			w.Header().Set("X-RateLimit-Resource", "search")
			if limited.Load() {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.WriteHeader(http.StatusForbidden)
				io.WriteString(w, `{"message":"API rate limit exceeded"}`)
				return
			}
			io.WriteString(w, `{"total_count":142,"incomplete_results":false,"items":[]}`)
		case strings.HasSuffix(req.URL.Path, "/stargazers/history"):
			fmt.Fprintf(w, `[{"week":%d,"total":120,"days":[100,20,0,0,0,0,0]}]`, sunday)
		default:
			io.WriteString(w, `{"id":3,"full_name":"me/new","created_at":"2026-10-20T00:00:00Z","stargazers_count":120,"forks_count":9}`)
		}
	}))
	defer gh.Close()
	r := &Runner{GitHub: &github.Client{BaseURL: gh.URL}, Now: func() time.Time { return time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC) }}
	result, err := r.Run(context.Background(), "me/new")
	if err != nil {
		t.Fatal(err)
	}
	if result.IssuesAndPRs == nil || *result.IssuesAndPRs != 142 || query != "repo:me/new created:>=2026-07-28" {
		t.Fatalf("issues %v, query %q", result.IssuesAndPRs, query)
	}
	if !strings.HasSuffix(result.Verdict, "So far it has 9 forks and 142 issues and pull requests, see below.") {
		t.Errorf("verdict %q", result.Verdict)
	}
	if result.Sources.GitHubRequests != 3 {
		t.Errorf("%d GitHub requests, want 3: summary, history, search", result.Sources.GitHubRequests)
	}

	limited.Store(true)
	result, err = r.Run(context.Background(), "me/new")
	if err != nil {
		t.Fatal(err)
	}
	if result.IssuesAndPRs != nil || !strings.HasSuffix(result.Verdict, "It has 9 forks, see below.") {
		t.Errorf("issues %v, verdict %q", result.IssuesAndPRs, result.Verdict)
	}
	// The limits say why the count is missing, with the search's own limit
	// instead of the hourly one.
	limits := strings.Join(result.Limits, "\n")
	if !strings.Contains(limits, "GitHub's search did not answer, so the recent issues and pull requests were not counted: "+
		"GitHub allows 10 searches a minute") || strings.Contains(limits, "60 requests an hour") {
		t.Errorf("limits:\n%s", limits)
	}
}

func TestCtrlCIsNotBlamedOnTheSearch(t *testing.T) {
	// After Ctrl-C the search never runs, so no limit blames it.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sunday := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC).Unix()
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/stargazers/history") {
			fmt.Fprintf(w, `[{"week":%d,"total":120,"days":[100,20,0,0,0,0,0]}]`, sunday)
			return
		}
		io.WriteString(w, `{"id":3,"full_name":"me/new","created_at":"2026-10-20T00:00:00Z","stargazers_count":120,"forks_count":9}`)
	}))
	defer gh.Close()
	r := &Runner{GitHub: &github.Client{BaseURL: gh.URL}, Progress: cancelAfter{"Reading star history", cancel},
		Now: func() time.Time { return time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC) }}
	result, err := r.Run(ctx, "me/new")
	if err != nil {
		t.Fatal(err)
	}
	limits := strings.Join(result.Limits, "\n")
	if result.IssuesAndPRs != nil || result.Sources.IssuesProblem != "" || strings.Contains(limits, "search") || !strings.Contains(limits, "Ctrl-C") {
		t.Errorf("issues %v, problem %q, limits:\n%s", result.IssuesAndPRs, result.Sources.IssuesProblem, limits)
	}
}

func TestUnknownIssueCountIsNullInJSON(t *testing.T) {
	// A script must be able to tell "no issues" from "not counted".
	data, _ := json.Marshal(Result{})
	if !strings.Contains(string(data), `"issues_and_prs_90_days":null`) {
		t.Errorf("json = %s", data)
	}
}
