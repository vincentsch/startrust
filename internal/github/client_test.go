package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// recorded is a set of real GitHub responses.
type recorded struct {
	Repo  json.RawMessage `json:"repo"`
	Pages []struct {
		Path string          `json:"path"`
		Link string          `json:"link"`
		Body json.RawMessage `json:"body"`
	} `json:"pages"`
}

func loadRecorded(t *testing.T) recorded {
	t.Helper()
	raw, err := os.ReadFile("testdata/glow-history.json")
	if err != nil {
		t.Fatal(err)
	}
	var r recorded
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// replay serves recorded responses. Link headers keep GitHub's real
// /repositories/{id}/... paths; only the host is swapped for the test server.
func replay(t *testing.T, r recorded) (*httptest.Server, *int32) {
	t.Helper()
	var requests int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt32(&requests, 1)
		if req.URL.Path == "/repos/charmbracelet/glow" {
			w.Write(r.Repo)
			return
		}
		for _, page := range r.Pages {
			if req.URL.RequestURI() == page.Path {
				if page.Link != "" {
					w.Header().Set("Link", strings.ReplaceAll(page.Link, "https://api.github.com", server.URL))
				}
				w.Write(page.Body)
				return
			}
		}
		t.Errorf("unexpected request %s", req.URL.RequestURI())
		http.NotFound(w, req)
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func TestStarHistoryFollowsRealGitHubLinks(t *testing.T) {
	r := loadRecorded(t)
	server, requests := replay(t, r)
	client := &Client{BaseURL: server.URL}

	var pagesSeen []int
	weeks, err := client.StarHistory(context.Background(), "charmbracelet/glow", func(page int) { pagesSeen = append(pagesSeen, page) })
	if err != nil {
		t.Fatalf("StarHistory: %v", err)
	}
	if len(pagesSeen) != len(r.Pages) || int(*requests) != len(r.Pages) {
		t.Fatalf("read %d pages with %d requests, want %d", len(pagesSeen), *requests, len(r.Pages))
	}
	total := 0
	for _, w := range weeks {
		total += w.Total
	}
	if total != 27818 {
		t.Errorf("total stars %d, want 27818 (the count GitHub reported that day)", total)
	}
}

func TestStarHistoryKeepsPagesReadBeforeAnError(t *testing.T) {
	r := loadRecorded(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.RequestURI() == r.Pages[0].Path {
			w.Header().Set("Link", strings.ReplaceAll(r.Pages[0].Link, "https://api.github.com", server.URL))
			w.Write(r.Pages[0].Body)
			return
		}
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(time.Hour).Unix()))
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"message":"API rate limit exceeded for 1.2.3.4."}`)
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL}
	weeks, err := client.StarHistory(context.Background(), "charmbracelet/glow", nil)
	if !IsKind(err, ErrRateLimited) {
		t.Fatalf("error = %v, want a rate limit error", err)
	}
	if len(weeks) != 30 {
		t.Errorf("kept %d weeks, want the 30 from page 1", len(weeks))
	}
	if !strings.Contains(err.Error(), "60 requests an hour") {
		t.Errorf("rate limit message should say how to get more requests: %q", err)
	}

	// With a token, telling the user to set one would be wrong.
	client = &Client{BaseURL: server.URL, Token: "test-token"}
	_, err = client.StarHistory(context.Background(), "charmbracelet/glow", nil)
	if msg := err.Error(); !strings.Contains(msg, "rate limit for this token is used up") || strings.Contains(msg, "GITHUB_TOKEN") {
		t.Errorf("rate limit message with a token: %q", msg)
	}
}

func TestLinkToAnotherHostIsRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Link", `<https://evil.example/repositories/1/stargazers/history?page=2>; rel="next"`)
		io.WriteString(w, `[]`)
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, Token: "ghp_secret"}
	_, err := client.StarHistory(context.Background(), "a/b", nil)
	if err == nil || !strings.Contains(err.Error(), "outside the GitHub API") {
		t.Fatalf("error = %v, want a refusal to follow the link", err)
	}
}

func TestRenamedRepoRedirectIsFollowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/repos/old-owner/old-name":
			http.Redirect(w, req, "/repositories/42", http.StatusMovedPermanently)
		case "/repositories/42":
			if req.Header.Get("Authorization") != "Bearer ghp_token" {
				t.Errorf("token missing after a same-host redirect")
			}
			io.WriteString(w, `{"id":42,"full_name":"new-owner/new-name","created_at":"2024-01-01T00:00:00Z","stargazers_count":5}`)
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, Token: "ghp_token"}
	repo, err := client.Repo(context.Background(), "old-owner/old-name")
	if err != nil {
		t.Fatal(err)
	}
	if repo.FullName != "new-owner/new-name" {
		t.Errorf("FullName = %q", repo.FullName)
	}
}

func TestAnonymousRequestsSendNoAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want none", got)
		}
		io.WriteString(w, `{"id":1,"full_name":"a/b","created_at":"2024-01-01T00:00:00Z"}`)
	}))
	defer server.Close()
	if _, err := (&Client{BaseURL: server.URL}).Repo(context.Background(), "a/b"); err != nil {
		t.Fatal(err)
	}
}

func TestErrorsAreClassifiedAndRedacted(t *testing.T) {
	cases := []struct {
		status  int
		body    string
		headers map[string]string
		want    ErrorKind
	}{
		{404, `{"message":"Not Found"}`, nil, ErrNotFound},
		{401, `{"message":"Bad credentials"}`, nil, ErrAuth},
		{403, `{"message":"Resource not accessible by personal access token ghp_abcdef123"}`, nil, ErrForbidden},
		{403, `{"message":"API rate limit exceeded"}`, map[string]string{"X-RateLimit-Remaining": "0"}, ErrRateLimited},
		{429, `{"message":"You have exceeded a secondary rate limit"}`, nil, ErrRateLimited},
		{500, `{"message":"boom"}`, nil, ErrUpstream},
	}
	for _, c := range cases {
		t.Run(fmt.Sprint(c.status, c.want), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				for k, v := range c.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(c.status)
				io.WriteString(w, c.body)
			}))
			defer server.Close()
			client := &Client{BaseURL: server.URL, Token: "ghp_abcdef123", Sleep: func(context.Context, time.Duration) error { return nil }}
			_, err := client.Repo(context.Background(), "a/b")
			if !IsKind(err, c.want) {
				t.Fatalf("error = %v, want kind %d", err, c.want)
			}
			if strings.Contains(err.Error(), "ghp_abcdef123") {
				t.Errorf("token leaked into error: %q", err)
			}
		})
	}
}

func TestServerErrorsAreRetried(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		io.WriteString(w, `{"id":1,"full_name":"a/b","created_at":"2024-01-01T00:00:00Z"}`)
	}))
	defer server.Close()
	var waits []time.Duration
	client := &Client{BaseURL: server.URL, Sleep: func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }}
	if _, err := client.Repo(context.Background(), "a/b"); err != nil {
		t.Fatal(err)
	}
	if calls != 3 || len(waits) != 2 {
		t.Errorf("calls = %d, waits = %v", calls, waits)
	}
}

func TestMaxRequestsStopsTheClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Link", fmt.Sprintf(`<http://%s/next?page=2>; rel="next"`, req.Host))
		io.WriteString(w, `[]`)
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, MaxRequests: 3}
	_, err := client.StarHistory(context.Background(), "a/b", nil)
	if !IsKind(err, ErrBudget) && !strings.Contains(fmt.Sprint(err), "loop") {
		t.Fatalf("error = %v", err)
	}
	if client.Requests() > 3 {
		t.Errorf("sent %d requests with a limit of 3", client.Requests())
	}
}

func TestStargazersDetectsGitHubsSilentEmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, `{"data":{"repository":{"stargazerCount":16,"stargazers":{"pageInfo":{"hasNextPage":false,"endCursor":null},"edges":[]}}}}`)
	}))
	defer server.Close()
	_, err := (&Client{BaseURL: server.URL, Token: "t"}).Stargazers(context.Background(), "a/b", "", nil)
	if !errors.Is(err, ErrStargazersHidden) {
		t.Fatalf("error = %v, want ErrStargazersHidden", err)
	}
}

func TestStargazersPagesWithCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(req.Body).Decode(&body)
		edge := `{"starredAt":"2026-10-01T10:00:00Z","node":{"login":"%s","createdAt":"2020-01-01T00:00:00Z","followers":{"totalCount":3},"repositories":{"totalCount":2}}}`
		if body.Variables["after"] == nil {
			fmt.Fprintf(w, `{"data":{"repository":{"stargazerCount":2,"stargazers":{"pageInfo":{"hasNextPage":true,"endCursor":"c1"},"edges":[`+edge+`]}}}}`, "first")
			return
		}
		fmt.Fprintf(w, `{"data":{"repository":{"stargazerCount":2,"stargazers":{"pageInfo":{"hasNextPage":false,"endCursor":"c2"},"edges":[`+edge+`]}}}}`, "second")
	}))
	defer server.Close()
	var cursors []string
	all, err := (&Client{BaseURL: server.URL, Token: "t"}).Stargazers(context.Background(), "a/b", "", func(_ []Stargazer, cursor string) { cursors = append(cursors, cursor) })
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[1].Login != "second" || all[0].PublicRepos != 2 || strings.Join(cursors, ",") != "c1,c2" {
		t.Errorf("got %+v, cursors %v", all, cursors)
	}
}

func TestParseName(t *testing.T) {
	cases := map[string]string{
		"junegunn/fzf":                                       "junegunn/fzf",
		"https://github.com/junegunn/fzf":                    "junegunn/fzf",
		"https://github.com/junegunn/fzf/":                   "junegunn/fzf",
		"https://github.com/junegunn/fzf/tree/master/bin":    "junegunn/fzf",
		"git@github.com:junegunn/fzf.git":                    "junegunn/fzf",
		"github.com/charmbracelet/glow":                      "charmbracelet/glow",
		"https://www.github.com/junegunn/fzf":                "junegunn/fzf",
		"https://github.com/junegunn/fzf#readme":             "junegunn/fzf",
		"https://github.com/junegunn/fzf?tab=readme-ov-file": "junegunn/fzf",
	}
	for input, want := range cases {
		if got, ok := ParseName(input); !ok || got != want {
			t.Errorf("ParseName(%q) = %q, %v; want %q", input, got, ok, want)
		}
	}
	for _, bad := range []string{"", "fzf", "a/b/c", "../x", "a b/c", "out/report.json/x"} {
		if got, ok := ParseName(bad); ok {
			t.Errorf("ParseName(%q) = %q, want an error", bad, got)
		}
	}
}

func TestHistoryPagesGrowsWithAge(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	if got := HistoryPages(now.AddDate(0, -1, 0), now); got != 1 {
		t.Errorf("one month old: %d pages", got)
	}
	// fzf is 13 years old and needed 23 pages.
	if got := HistoryPages(time.Date(2013, 10, 23, 0, 0, 0, 0, time.UTC), now); got != 23 {
		t.Errorf("fzf: %d pages, want 23", got)
	}
}

func TestIssuesAndPullRequestsSince(t *testing.T) {
	var query string
	incomplete := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		query = req.URL.Query().Get("q")
		w.Header().Set("X-RateLimit-Resource", "search")
		w.Header().Set("X-RateLimit-Remaining", "9")
		fmt.Fprintf(w, `{"total_count":61,"incomplete_results":%v,"items":[]}`, incomplete)
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL}
	since := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	n, err := client.IssuesAndPullRequestsSince(context.Background(), "junegunn/fzf", since)
	if err != nil || n != 61 {
		t.Fatalf("count = %d, %v", n, err)
	}
	// Without is:issue or is:pr, GitHub counts both.
	if query != "repo:junegunn/fzf created:>=2026-07-28" {
		t.Errorf("query = %q", query)
	}
	incomplete = true
	if _, err := client.IssuesAndPullRequestsSince(context.Background(), "junegunn/fzf", since); err == nil {
		t.Errorf("a search that timed out must not count")
	}
}

func TestSearchLimitIsNotTheHourlyLimit(t *testing.T) {
	// A search answer carries the search limit (10 a minute). Taking it
	// for the hourly limit would warn about requests running out.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/search/") {
			w.Header().Set("X-RateLimit-Resource", "search")
			w.Header().Set("X-RateLimit-Remaining", "9")
			io.WriteString(w, `{"total_count":3,"incomplete_results":false}`)
			return
		}
		w.Header().Set("X-RateLimit-Resource", "core")
		w.Header().Set("X-RateLimit-Remaining", "4321")
		io.WriteString(w, `{"id":1,"full_name":"a/b"}`)
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL}
	if _, err := client.Repo(context.Background(), "a/b"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.IssuesAndPullRequestsSince(context.Background(), "a/b", time.Now()); err != nil {
		t.Fatal(err)
	}
	if rate, ok := client.RateLimit(); !ok || rate.Remaining != 4321 {
		t.Errorf("rate limit = %+v, want 4321 left of the hourly limit", rate)
	}
}

func TestRateLimitComesFromTheHeaders(t *testing.T) {
	// Before GitHub sends its numbers, nothing is known, which is not the
	// same as no requests left.
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, `{"id":1,"full_name":"a/b"}`)
	}))
	defer plain.Close()
	client := &Client{BaseURL: plain.URL}
	if _, err := client.Repo(context.Background(), "a/b"); err != nil {
		t.Fatal(err)
	}
	if rate, ok := client.RateLimit(); ok {
		t.Errorf("rate limit %+v without any header", rate)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Remaining", "48")
		w.Header().Set("X-RateLimit-Reset", "1793710800")
		io.WriteString(w, `{"id":1,"full_name":"a/b"}`)
	}))
	defer server.Close()
	client = &Client{BaseURL: server.URL}
	if _, err := client.Repo(context.Background(), "a/b"); err != nil {
		t.Fatal(err)
	}
	want := RateLimit{Limit: 60, Remaining: 48, ResetsAt: time.Unix(1793710800, 0).UTC()}
	if rate, ok := client.RateLimit(); !ok || rate != want {
		t.Errorf("rate limit = %+v, want %+v", rate, want)
	}
}
