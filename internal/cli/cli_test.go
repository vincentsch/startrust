package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/startrust/internal/check"
	"github.com/vincentsch/startrust/internal/github"
)

var update = flag.Bool("update", false, "rewrite golden files")

// testNow is the day the fixtures were captured.
var testNow = time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC)

type fixture struct {
	Repo struct {
		ID        int64     `json:"id"`
		FullName  string    `json:"full_name"`
		CreatedAt time.Time `json:"created_at"`
		Stars     int       `json:"stargazers_count"`
		Forks     int       `json:"forks_count"`
		Watchers  int       `json:"subscribers_count"`
	} `json:"repo"`
	// IssuesAndPRs is what GitHub's search counted for the last 90 days.
	IssuesAndPRs int `json:"issues_and_prs_90_days"`
	Accounts     []struct {
		Day            time.Time `json:"day"`
		Archived       int       `json:"archived"`
		WindowRecorded float64   `json:"window_recorded"`
		Checked        int       `json:"checked"`
		OneOff         int       `json:"one_off"`
	} `json:"accounts"`
	Weeks []json.RawMessage `json:"weeks"`
}

func loadFixture(t *testing.T, name string) fixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "stars", "testdata", strings.ReplaceAll(name, "/", "__")+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// fakeGitHub serves the captured repos the way GitHub does: 30 weeks per
// page, newest first, with Link headers that use /repositories/{id} paths.
func fakeGitHub(t *testing.T, names ...string) (*httptest.Server, *[]string) {
	t.Helper()
	return fakeRenamedGitHub(t, nil, names...)
}

// fakeRenamedGitHub is fakeGitHub with renamed repos: each old name in
// renames is forwarded to the repo with the new name, the way GitHub
// forwards a renamed repo's old names to /repositories/{id}.
func fakeRenamedGitHub(t *testing.T, renames map[string]string, names ...string) (*httptest.Server, *[]string) {
	t.Helper()
	fixtures := map[string]fixture{}
	byID := map[string]fixture{}
	for _, name := range names {
		f := loadFixture(t, name)
		fixtures[strings.ToLower(f.Repo.FullName)] = f
		byID[fmt.Sprint(f.Repo.ID)] = f
	}
	var seen []string
	var server *httptest.Server
	historyPath := regexp.MustCompile(`^/(?:repos/([^/]+/[^/]+)|repositories/(\d+))/stargazers/history$`)
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		seen = append(seen, req.URL.RequestURI())
		if m := historyPath.FindStringSubmatch(req.URL.Path); m != nil {
			f, ok := fixtures[strings.ToLower(m[1])]
			if m[2] != "" {
				f, ok = byID[m[2]]
			}
			if !ok {
				http.NotFound(w, req)
				return
			}
			page := 1
			fmt.Sscan(req.URL.Query().Get("page"), &page)
			from, to := (page-1)*30, min(page*30, len(f.Weeks))
			if page*30 < len(f.Weeks) {
				w.Header().Set("Link", fmt.Sprintf(`<%s/repositories/%d/stargazers/history?per_page=30&page=%d>; rel="next"`, server.URL, f.Repo.ID, page+1))
			}
			io.WriteString(w, "["+string(bytes.Join(rawSlice(f.Weeks[from:to]), []byte(",")))+"]")
			return
		}
		if req.URL.Path == "/search/issues" {
			// The query is "repo:owner/name created:>=day".
			name, _, _ := strings.Cut(strings.TrimPrefix(req.URL.Query().Get("q"), "repo:"), " ")
			w.Header().Set("X-RateLimit-Resource", "search")
			fmt.Fprintf(w, `{"total_count":%d,"incomplete_results":false,"items":[]}`, fixtures[strings.ToLower(name)].IssuesAndPRs)
			return
		}
		if current, ok := renames[strings.ToLower(strings.TrimPrefix(req.URL.Path, "/repos/"))]; ok {
			http.Redirect(w, req, fmt.Sprintf("/repositories/%d", fixtures[strings.ToLower(current)].Repo.ID), http.StatusMovedPermanently)
			return
		}
		f, ok := fixtures[strings.ToLower(strings.TrimPrefix(req.URL.Path, "/repos/"))]
		if id, found := strings.CutPrefix(req.URL.Path, "/repositories/"); found {
			f, ok = byID[id]
		}
		if ok {
			json.NewEncoder(w).Encode(map[string]any{
				"id": f.Repo.ID, "full_name": f.Repo.FullName, "html_url": "https://github.com/" + f.Repo.FullName,
				"created_at": f.Repo.CreatedAt, "stargazers_count": f.Repo.Stars, "forks_count": f.Repo.Forks,
				"subscribers_count": f.Repo.Watchers,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"message":"Not Found"}`)
	}))
	t.Cleanup(server.Close)
	return server, &seen
}

func rawSlice(in []json.RawMessage) [][]byte {
	out := make([][]byte, len(in))
	for i, r := range in {
		out[i] = r
	}
	return out
}

// archiveAnswer is what the fake GH Archive returns for one day.
type archiveAnswer struct {
	archived, checked, oneOff int
	// windowRecorded is 1 when GH Archive recorded as many stars a day in
	// the weeks around the day as in its week. Zero counts as 1.
	windowRecorded float64
}

// fakeArchive answers cohort queries with fixed numbers per day. Days not
// listed have no archive data.
func fakeArchive(t *testing.T, answers map[string]archiveAnswer) *httptest.Server {
	t.Helper()
	dayPattern := regexp.MustCompile(`toDate\('(\d{4}-\d{2}-\d{2})'\)`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		m := dayPattern.FindSubmatch(body)
		a := answers[string(m[1])]
		if a.windowRecorded == 0 {
			a.windowRecorded = 1
		}
		// The query returns the stars across GitHub in the 61-day window
		// and in the week around the day.
		fmt.Fprintf(w, `{"data":[["%d","%d","%d","%d","7000"]]}`, a.archived, a.checked, a.oneOff, int(a.windowRecorded*61000))
	}))
	t.Cleanup(server.Close)
	return server
}

// fixtureArchive answers with the GH Archive numbers saved in a fixture.
func fixtureArchive(t *testing.T, name string) *httptest.Server {
	t.Helper()
	answers := map[string]archiveAnswer{}
	for _, c := range loadFixture(t, name).Accounts {
		answers[c.Day.Format("2006-01-02")] = archiveAnswer{c.Archived, c.Checked, c.OneOff, c.WindowRecorded}
	}
	return fakeArchive(t, answers)
}

type runResult struct {
	code   int
	stdout string
	stderr string
}

func run(t *testing.T, services Services, args ...string) runResult {
	t.Helper()
	return runContext(t, context.Background(), services, args...)
}

// runContext runs the CLI with a context the test can cancel, the way
// Ctrl-C does.
func runContext(t *testing.T, ctx context.Context, services Services, args ...string) runResult {
	t.Helper()
	if services.Getenv == nil {
		services.Getenv = func(string) string { return "" }
	}
	if services.Now == nil {
		services.Now = func() time.Time { return testNow }
	}
	// Never ask the real GitHub CLI for a token in tests.
	if services.FindToken == nil {
		services.FindToken = func(context.Context) github.Token { return github.Token{} }
	}
	off := false
	services.Terminal = &off
	var stdout, stderr bytes.Buffer
	app := NewApp(services)
	app.Root().SetContext(ctx)
	code := app.Run(args, &stdout, &stderr)
	return runResult{code, stdout.String(), stderr.String()}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(path, []byte(got), 0o644)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden file %s; run go test ./internal/cli -update", path)
	}
	if string(want) != got {
		t.Errorf("output differs from %s (run with -update to accept):\n%s", path, got)
	}
}

// stable removes the parts of the output that change between runs.
func stable(s string) string {
	s = regexp.MustCompile(`Checked \d{4}-\d{2}-\d{2} \d{2}:\d{2} \S+`).ReplaceAllString(s, "Checked <time>")
	return s
}

// flagged is a repo that research flagged for bought stars. Its name is
// made up: GitHub user names cannot contain an underscore.
const flagged = "flagged_a/repo"

func TestCheckFlaggedRepo(t *testing.T) {
	gh, _ := fakeGitHub(t, flagged)
	services := Services{GitHubURL: gh.URL, ArchiveURL: fixtureArchive(t, flagged).URL}
	cacheDir := t.TempDir()

	out := run(t, services, "check", flagged, "--cache-dir", cacheDir)
	if out.code != 0 {
		t.Fatalf("exit %d: %s", out.code, out.stderr)
	}
	golden(t, "check-flagged.txt", stable(out.stdout))
	if !strings.HasPrefix(strings.SplitN(out.stdout, "\n", 4)[3], "Unusual star pattern.") {
		t.Errorf("the verdict must come right after the header")
	}

	out = run(t, services, "check", flagged, "--json", "--cache-dir", cacheDir)
	var summary check.Result
	if err := json.Unmarshal([]byte(out.stdout), &summary); err != nil {
		t.Fatalf("--json: %v\n%s", err, out.stdout)
	}
	if summary.Risk != check.RiskHigh || summary.UnusualSpikes != 1 || summary.Burst == nil || summary.Details != nil {
		t.Errorf("summary = %+v", summary)
	}
	if len(out.stdout) > 8000 {
		t.Errorf("--json summary is %d bytes; it should stay compact", len(out.stdout))
	}

	out = run(t, services, "check", flagged, "--json", "--details", "--cache-dir", cacheDir)
	var full check.Result
	json.Unmarshal([]byte(out.stdout), &full)
	if full.Details == nil || full.Details.History.Total() != loadFixture(t, flagged).Repo.Stars {
		t.Errorf("--json --details must include the daily history")
	}
}

func TestCheckFzf(t *testing.T) {
	gh, _ := fakeGitHub(t, "junegunn/fzf")
	services := Services{GitHubURL: gh.URL, ArchiveURL: fixtureArchive(t, "junegunn/fzf").URL}
	out := run(t, services, "check", "junegunn/fzf", "--cache-dir", t.TempDir())
	if out.code != 0 {
		t.Fatalf("exit %d: %s", out.code, out.stderr)
	}
	golden(t, "check-fzf.txt", stable(out.stdout))
}

func TestFewForksDoNotChangeTheRisk(t *testing.T) {
	// Flagged repo I got most of its stars at its launch, so without the
	// account check nothing could judge it. It has 4 forks for 1,132 stars,
	// but few forks alone mean nothing, so the risk stays unknown.
	const name = "flagged_i/repo"
	gh, _ := fakeGitHub(t, name)
	out := run(t, Services{GitHubURL: gh.URL}, "check", name, "--no-archive", "--cache-dir", t.TempDir())
	if out.code != 0 {
		t.Fatalf("exit %d: %s", out.code, out.stderr)
	}
	golden(t, "check-few-forks.txt", stable(out.stdout))
	out = run(t, Services{GitHubURL: gh.URL}, "check", name, "--no-archive", "--json", "--cache-dir", t.TempDir())
	var r check.Result
	if err := json.Unmarshal([]byte(out.stdout), &r); err != nil || r.Risk != check.RiskUnknown || r.Headline != "not enough data" {
		t.Errorf("risk %s, headline %q: %v", r.Risk, r.Headline, err)
	}
	if strings.Contains(out.stdout, "few_forks") {
		t.Errorf("the JSON still has few_forks:\n%s", out.stdout)
	}
}

func TestCheckLaunchWithoutAccountData(t *testing.T) {
	// A month-old repo, and GH Archive has 5 of the launch day's 261
	// stargazers. Nothing could be judged, so the risk is unknown, and the
	// badge must not sound like a pass.
	gh, _ := fakeGitHub(t, "vincentsch/explainroo")
	services := Services{GitHubURL: gh.URL, ArchiveURL: fixtureArchive(t, "vincentsch/explainroo").URL}
	dir := t.TempDir()
	out := run(t, services, "check", "vincentsch/explainroo", "--cache-dir", dir)
	if out.code != 0 {
		t.Fatalf("exit %d: %s", out.code, out.stderr)
	}
	golden(t, "check-explainroo.txt", stable(out.stdout))
	out = run(t, Services{}, "badge", "vincentsch/explainroo", "--cache-dir", dir)
	if !strings.Contains(out.stdout, `"message": "too new to judge"`) {
		t.Errorf("badge = %s", out.stdout)
	}
	// --fail-on cannot fail on a repo nobody could judge, and says so.
	out = run(t, services, "check", "vincentsch/explainroo", "--fail-on", "LOW", "--cache-dir", dir)
	if out.code != 0 || !strings.Contains(out.stderr, "could not judge vincentsch/explainroo (risk unknown), so --fail-on low does not apply") {
		t.Errorf("exit %d, stderr %q", out.code, out.stderr)
	}
}

func TestStoppedHistoryIsNotReassuring(t *testing.T) {
	// With only two requests, the check reads the newest 30 weeks, after
	// the campaign. That part looks quiet, which must not read as a clean
	// result.
	gh, _ := fakeGitHub(t, flagged)
	dir := t.TempDir()
	out := run(t, Services{GitHubURL: gh.URL}, "check", flagged, "--no-archive", "--max-requests", "2", "--json", "--cache-dir", dir)
	var r check.Result
	if err := json.Unmarshal([]byte(out.stdout), &r); err != nil {
		t.Fatalf("exit %d: %v\n%s", out.code, err, out.stderr)
	}
	if !r.HistoryPartial || r.Risk != check.RiskUnknown || !strings.HasPrefix(r.Verdict, "Not enough data to judge. StarTrust read only part of the star history") {
		t.Errorf("partial %v, risk %s, verdict %q", r.HistoryPartial, r.Risk, r.Verdict)
	}
	out = run(t, Services{}, "badge", flagged, "--cache-dir", dir)
	if !strings.Contains(out.stdout, `"message": "not enough data"`) {
		t.Errorf("badge = %s", out.stdout)
	}
}

func TestCheckWithoutArchiveFallsBackToTiming(t *testing.T) {
	gh, _ := fakeGitHub(t, flagged)
	out := run(t, Services{GitHubURL: gh.URL}, "check", flagged, "--no-archive", "--json", "--cache-dir", t.TempDir())
	var r check.Result
	json.Unmarshal([]byte(out.stdout), &r)
	if r.Risk != check.RiskMedium || r.Verdict != "One unusual spike and most stars in one burst, see below." {
		t.Errorf("risk %s, verdict %q", r.Risk, r.Verdict)
	}
}

func TestArchiveOutageStillGivesAResult(t *testing.T) {
	gh, _ := fakeGitHub(t, flagged)
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(500)
		io.WriteString(w, "Code: 201. DB::Exception: Quota for user `play` exceeded (QUOTA_EXCEEDED)")
	}))
	defer down.Close()
	out := run(t, Services{GitHubURL: gh.URL, ArchiveURL: down.URL}, "check", flagged, "--cache-dir", t.TempDir())
	if out.code != 0 {
		t.Fatalf("exit %d: %s", out.code, out.stderr)
	}
	if !strings.Contains(out.stdout, "One unusual spike") || !strings.Contains(out.stdout, "hourly limit") {
		t.Errorf("want the timing verdict and the reason the account check failed:\n%s", out.stdout)
	}
}

func TestFailOnExitsWithPolicyCode(t *testing.T) {
	gh, _ := fakeGitHub(t, flagged, "junegunn/fzf")
	services := Services{GitHubURL: gh.URL, ArchiveURL: fixtureArchive(t, flagged).URL}
	if out := run(t, services, "check", flagged, "--fail-on", "high", "--cache-dir", t.TempDir()); out.code != ExitPolicy {
		t.Errorf("--fail-on high: exit %d, want %d", out.code, ExitPolicy)
	}
	if out := run(t, services, "check", "junegunn/fzf", "--fail-on", "medium", "--no-archive", "--cache-dir", t.TempDir()); out.code != 0 {
		t.Errorf("fzf --fail-on medium: exit %d", out.code)
	}
	if out := run(t, services, "check", "junegunn/fzf", "--fail-on", "unknown"); out.code != 1 || !strings.Contains(out.stderr, "read `risk` from --json") {
		t.Errorf("--fail-on unknown: exit %d, stderr %q", out.code, out.stderr)
	}
	if out := run(t, services, "check", "junegunn/fzf", "--fail-on", "sometimes"); out.code != 1 {
		t.Errorf("bad --fail-on: exit %d", out.code)
	}
}

func TestUnknownRepoExitsNotFound(t *testing.T) {
	gh, _ := fakeGitHub(t)
	out := run(t, Services{GitHubURL: gh.URL}, "check", "nobody/nothing", "--cache-dir", t.TempDir())
	if out.code != 5 || !strings.Contains(out.stderr, "no public repository called nobody/nothing") {
		t.Errorf("exit %d, stderr %q", out.code, out.stderr)
	}
}

func TestReportPrefersExistingFiles(t *testing.T) {
	gh, _ := fakeGitHub(t, "vincentsch/explainroo")
	archive := fakeArchive(t, nil)
	dir := t.TempDir()
	services := Services{GitHubURL: gh.URL, ArchiveURL: archive.URL}
	// out/report.json has one slash, like owner/repo. It is a file here.
	if err := os.MkdirAll(filepath.Join(dir, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	saved := filepath.Join(dir, "out", "report.json")
	if out := run(t, services, "check", "vincentsch/explainroo", "--json", "--details", "--out", saved, "--cache-dir", dir); out.code != 0 {
		t.Fatalf("exit %d: %s", out.code, out.stderr)
	}
	t.Chdir(dir)
	out := run(t, Services{}, "report", "out/report.json", "--cache-dir", filepath.Join(dir, "empty"))
	if out.code != 0 || !strings.Contains(out.stdout, "vincentsch/explainroo") || !strings.Contains(out.stdout, "Stars per day") {
		t.Errorf("exit %d\n%s%s", out.code, out.stdout, out.stderr)
	}
	out = run(t, Services{}, "report", "vincentsch/explainroo", "--markdown", "--cache-dir", dir)
	if out.code != 0 || !strings.Contains(out.stdout, "## Stars of [vincentsch/explainroo]") || !strings.Contains(out.stdout, "```text") {
		t.Errorf("markdown from cache: exit %d\n%s%s", out.code, out.stdout, out.stderr)
	}
	out = run(t, Services{}, "report", "missing/report.json")
	if out.code != 5 || !strings.Contains(out.stderr, "no file called") {
		t.Errorf("missing file: exit %d %s", out.code, out.stderr)
	}
}

func TestBadgeShowsTheHeadline(t *testing.T) {
	// The check decides the headline; the badge only shows it, in one
	// neutral color whatever the risk.
	for _, r := range []check.Result{
		{Risk: check.RiskLow, Headline: "no unusual pattern"},
		{Risk: check.RiskHigh, Headline: "unusual pattern"},
		{Risk: check.RiskUnknown, Headline: "too new to judge"},
	} {
		badge := BadgeFor(&r)
		if badge.Message != r.Headline || badge.Color != "informational" || !strings.Contains(badge.Label, "experimental") {
			t.Errorf("%s: %+v", r.Risk, badge)
		}
	}
}

func TestBadgeFromCache(t *testing.T) {
	gh, _ := fakeGitHub(t, "junegunn/fzf")
	dir := t.TempDir()
	run(t, Services{GitHubURL: gh.URL}, "check", "junegunn/fzf", "--no-archive", "--cache-dir", dir)
	badgeFile := filepath.Join(dir, "badge.json")
	out := run(t, Services{}, "badge", "junegunn/fzf", "--out", badgeFile, "--cache-dir", dir)
	if out.code != 0 {
		t.Fatalf("exit %d: %s", out.code, out.stderr)
	}
	data, _ := os.ReadFile(badgeFile)
	if !strings.Contains(string(data), `"message": "no unusual timing"`) {
		t.Errorf("badge = %s", data)
	}
}

func TestFormerNameCoversTheWholeHistory(t *testing.T) {
	// Pretend fzf was called old/fzf until now: GitHub forwards the old
	// name, and GH Archive has its busy days only under that name.
	gh, seen := fakeRenamedGitHub(t, map[string]string{"old/fzf": "junegunn/fzf"}, "junegunn/fzf")
	underOldName := fixtureArchive(t, "junegunn/fzf")
	archive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(body), "'old/fzf'") {
			io.WriteString(w, `{"data":[["0","0","0","61000","7000"]]}`)
			return
		}
		proxy, _ := http.NewRequest(http.MethodPost, underOldName.URL, bytes.NewReader(body))
		res, err := http.DefaultClient.Do(proxy)
		if err != nil {
			t.Error(err)
			return
		}
		defer res.Body.Close()
		io.Copy(w, res.Body)
	}))
	defer archive.Close()
	services := Services{GitHubURL: gh.URL, ArchiveURL: archive.URL}
	decode := func(out runResult) check.Result {
		t.Helper()
		var r check.Result
		if err := json.Unmarshal([]byte(out.stdout), &r); err != nil {
			t.Fatalf("exit %d: %v\n%s", out.code, err, out.stderr)
		}
		return r
	}

	// Under the current name alone, the check suspects a rename and says
	// what to run.
	r := decode(run(t, services, "check", "junegunn/fzf", "--json", "--cache-dir", t.TempDir()))
	if !r.RenameSuspected || r.AccountsChecked != "none" || len(r.FormerNames) != 0 ||
		!strings.Contains(strings.Join(r.Limits, "\n"), "run `startrust check junegunn/fzf --former-name OLD-OWNER/OLD-NAME` with that name") {
		t.Errorf("rename suspected %v, accounts %s, former names %v, limits:\n%s", r.RenameSuspected, r.AccountsChecked, r.FormerNames, strings.Join(r.Limits, "\n"))
	}

	// With --former-name, one check covers every busy day, and the report
	// says which name GitHub forwards.
	out := run(t, services, "check", "junegunn/fzf", "--former-name", "https://github.com/old/fzf", "--cache-dir", t.TempDir())
	header, _, _ := strings.Cut(out.stdout, "\n\n")
	if out.code != 0 || !strings.HasSuffix(strings.Join(strings.Fields(header), " "),
		"https://github.com/junegunn/fzf GitHub forwards old/fzf to this repo. GH Archive was also searched under that name.") ||
		!strings.Contains(out.stdout, "2% one-off accounts") {
		t.Errorf("exit %d\n%s%s", out.code, out.stdout, out.stderr)
	}
	r = decode(run(t, services, "check", "junegunn/fzf", "--former-name", "old/fzf", "--json", "--cache-dir", t.TempDir()))
	if r.RenameSuspected || r.AccountsChecked != "all" || len(r.FormerNames) != 1 || r.FormerNames[0] != "old/fzf" {
		t.Errorf("rename suspected %v, accounts %s, former names %v", r.RenameSuspected, r.AccountsChecked, r.FormerNames)
	}

	// Checking the old name gives the same result under the current name,
	// and says that GitHub forwarded it.
	r = decode(run(t, services, "check", "old/fzf", "--json", "--cache-dir", t.TempDir()))
	if r.Repository != "junegunn/fzf" || len(r.FormerNames) != 1 || r.FormerNames[0] != "old/fzf" || r.AccountsChecked != "all" {
		t.Errorf("repository %s, former names %v, accounts %s", r.Repository, r.FormerNames, r.AccountsChecked)
	}

	// A name GitHub does not forward to the repo is a usage error, found
	// before the check reads the history.
	*seen = nil
	out = run(t, services, "check", "junegunn/fzf", "--former-name", "nobody/nothing", "--cache-dir", t.TempDir())
	if out.code != 1 || !strings.Contains(out.stderr, "--former-name nobody/nothing: GitHub has no repo by that name") ||
		strings.Contains(strings.Join(*seen, "\n"), "history") {
		t.Errorf("exit %d, requests %v, stderr %q", out.code, *seen, out.stderr)
	}
	// Mistakes in the flag itself stop before any request.
	*seen = nil
	for _, args := range [][]string{
		{"check", "junegunn/fzf", "--former-name", "not a repo"},
		{"check", "junegunn/fzf", "--former-name", "old/fzf", "--no-archive"},
	} {
		if out := run(t, services, append(args, "--cache-dir", t.TempDir())...); out.code != 1 || !strings.Contains(out.stderr, "--former-name") {
			t.Errorf("%v: exit %d, stderr %q", args, out.code, out.stderr)
		}
	}
	if len(*seen) != 0 {
		t.Errorf("flag mistakes sent requests: %v", *seen)
	}
}

func TestCompare(t *testing.T) {
	gh, _ := fakeGitHub(t, flagged, "junegunn/fzf")
	out := run(t, Services{GitHubURL: gh.URL}, "compare", flagged, "junegunn/fzf", "nobody/nothing", "--no-archive", "--cache-dir", t.TempDir())
	// The table shows the two repos that worked; the exit code says that
	// one repo does not exist.
	if out.code != 5 || !strings.Contains(out.stderr, "nobody/nothing") {
		t.Fatalf("exit %d: %s", out.code, out.stderr)
	}
	golden(t, "compare.txt", out.stdout)
}

func TestCacheListAndPrune(t *testing.T) {
	gh, _ := fakeGitHub(t, "junegunn/fzf")
	dir := t.TempDir()
	for _, at := range []time.Time{testNow.AddDate(0, -3, 0), testNow} {
		at := at
		run(t, Services{GitHubURL: gh.URL, Now: func() time.Time { return at }}, "check", "junegunn/fzf", "--no-archive", "--cache-dir", dir)
	}
	out := run(t, Services{}, "cache", "list", "--json", "--cache-dir", dir)
	var list cacheList
	json.Unmarshal([]byte(out.stdout), &list)
	if len(list.Checks) != 2 {
		t.Fatalf("list = %s", out.stdout)
	}
	if out := run(t, Services{}, "cache", "prune", "--cache-dir", dir); out.code != 1 || !strings.Contains(out.stderr, "--confirm") {
		t.Errorf("prune without --confirm: exit %d %s", out.code, out.stderr)
	}
	if out := run(t, Services{}, "cache", "prune", "--dry-run", "--cache-dir", dir); out.code != 0 || !strings.Contains(out.stdout, "Would delete 1 file (") {
		t.Errorf("dry run: exit %d %s%s", out.code, out.stdout, out.stderr)
	}
	if out := run(t, Services{}, "cache", "prune", "--confirm", "--cache-dir", dir); out.code != 0 {
		t.Errorf("prune: exit %d %s", out.code, out.stderr)
	}
	out = run(t, Services{}, "cache", "list", "--json", "--cache-dir", dir)
	json.Unmarshal([]byte(out.stdout), &list)
	if len(list.Checks) != 1 {
		t.Errorf("after prune: %s", out.stdout)
	}
}

func TestHelpSaysThereIsNoConfigFile(t *testing.T) {
	out := run(t, Services{}, "--help")
	if !strings.Contains(out.stdout, "--config string   Ignored, because StarTrust has no config file") {
		t.Errorf("--help does not say that --config is ignored:\n%s", out.stdout)
	}
	// The flag still works, so scripts that pass it keep running.
	dir := t.TempDir()
	if out := run(t, Services{}, "cache", "list", "--config", filepath.Join(dir, "missing.yaml"), "--cache-dir", dir); out.code != 0 {
		t.Errorf("exit %d, stderr %q", out.code, out.stderr)
	}
}

func TestHelpHasExamples(t *testing.T) {
	out := run(t, Services{}, "check", "--help")
	for _, want := range []string{"startrust check junegunn/fzf", "--fail-on", "--explain", "--deep", "--no-archive"} {
		if !strings.Contains(out.stdout, want) {
			t.Errorf("check --help is missing %q", want)
		}
	}
}

// TestJSONLayout pins the --json --details layout with a full saved check.
// Scripts and the conformance run read this file, so a change here is a
// change to the public JSON.
func TestJSONLayout(t *testing.T) {
	gh, _ := fakeGitHub(t, "vincentsch/explainroo")
	archive := fakeArchive(t, map[string]archiveAnswer{"2026-10-01": {archived: 5, checked: 5, oneOff: 2}})
	out := run(t, Services{GitHubURL: gh.URL, ArchiveURL: archive.URL}, "check", "vincentsch/explainroo", "--json", "--details", "--cache-dir", t.TempDir())
	if out.code != 0 {
		t.Fatalf("exit %d: %s", out.code, out.stderr)
	}
	golden(t, "check-saved.json", out.stdout)
}

func TestPruneAlwaysNeedsConfirmation(t *testing.T) {
	// Even an empty cache: scripts must get the same exit code either way.
	out := run(t, Services{}, "cache", "prune", "--cache-dir", t.TempDir())
	if out.code != 1 || !strings.Contains(out.stderr, "--confirm") {
		t.Errorf("exit %d, stderr %q", out.code, out.stderr)
	}
}

func TestRejectedTokenFallsBackToAnonymous(t *testing.T) {
	fixtures, _ := fakeGitHub(t, "junegunn/fzf")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"message":"Bad credentials"}`)
			return
		}
		proxy, _ := http.NewRequest(req.Method, fixtures.URL+req.URL.RequestURI(), nil)
		res, err := http.DefaultClient.Do(proxy)
		if err != nil {
			t.Error(err)
			return
		}
		defer res.Body.Close()
		for k, v := range res.Header {
			w.Header()[k] = v
		}
		body, _ := io.ReadAll(res.Body)
		w.Write([]byte(strings.ReplaceAll(string(body), fixtures.URL, "http://"+req.Host)))
	}))
	defer server.Close()
	services := Services{GitHubURL: server.URL, FindToken: func(context.Context) github.Token { return github.Token{Value: "expired", Source: "GH_TOKEN"} }}
	out := run(t, services, "check", "junegunn/fzf", "--no-archive", "--cache-dir", t.TempDir())
	// The report names the token that failed and says the check ran
	// without one.
	if out.code != 0 || !strings.Contains(out.stdout, "GitHub rejected the token in GH_TOKEN (HTTP 401)") ||
		!strings.Contains(out.stdout, "Set a new token in GH_TOKEN") || !strings.Contains(out.stdout, "Used no GitHub token") {
		t.Errorf("exit %d\n%s%s", out.code, out.stdout, out.stderr)
	}
	out = run(t, services, "check", "junegunn/fzf", "--no-archive", "--json", "--cache-dir", t.TempDir())
	var r check.Result
	if err := json.Unmarshal([]byte(out.stdout), &r); err != nil || r.Sources.TokenSource != "none" || r.Sources.TokenProblem == "" {
		t.Errorf("token_source %q, token_problem %q: %v", r.Sources.TokenSource, r.Sources.TokenProblem, err)
	}
}

func TestReportSaysWhichTokenWasUsed(t *testing.T) {
	gh, _ := fakeGitHub(t, "vincentsch/explainroo")
	for _, tc := range []struct {
		source, words string
	}{
		{"GITHUB_TOKEN", "Used the token in GITHUB_TOKEN."},
		{"GH_TOKEN", "Used the token in GH_TOKEN."},
		{"gh", "Used your GitHub CLI login, 5,000 requests an hour."},
		{"", "Used no GitHub token, 60 requests an hour."},
	} {
		token := github.Token{Source: tc.source}
		if tc.source != "" {
			token.Value = "token-from-" + tc.source
		}
		services := Services{GitHubURL: gh.URL, FindToken: func(context.Context) github.Token { return token }}
		out := run(t, services, "check", "vincentsch/explainroo", "--no-archive", "--cache-dir", t.TempDir())
		if text := strings.Join(strings.Fields(out.stdout), " "); !strings.Contains(text, tc.words) {
			t.Errorf("%q: the report does not say %q:\n%s", tc.source, tc.words, out.stdout)
		}
		out = run(t, services, "check", "vincentsch/explainroo", "--no-archive", "--json", "--cache-dir", t.TempDir())
		want := tc.source
		if want == "" {
			want = "none"
		}
		if !strings.Contains(out.stdout, `"token_source": "`+want+`"`) || strings.Contains(out.stdout, `"token":`) {
			t.Errorf("%q: JSON sources:\n%s", tc.source, out.stdout)
		}
	}
}

func TestNoTokenNeverLooksForOne(t *testing.T) {
	gh, seen := fakeGitHub(t, "vincentsch/explainroo")
	services := Services{GitHubURL: gh.URL, FindToken: func(context.Context) github.Token {
		t.Error("--no-token must not look for a token")
		return github.Token{Value: "secret", Source: "gh"}
	}}
	out := run(t, services, "check", "vincentsch/explainroo", "--no-archive", "--no-token", "--json", "--cache-dir", t.TempDir())
	var r check.Result
	if err := json.Unmarshal([]byte(out.stdout), &r); err != nil || r.Sources.TokenSource != "none" {
		t.Errorf("exit %d, token_source %q: %v\n%s", out.code, r.Sources.TokenSource, err, out.stderr)
	}
	out = run(t, services, "compare", "vincentsch/explainroo", "vincentsch/explainroo", "--no-archive", "--no-token", "--json", "--cache-dir", t.TempDir())
	if out.code != 0 || strings.Contains(out.stdout, `"token_source": "gh"`) {
		t.Errorf("compare --no-token: exit %d\n%s", out.code, out.stdout)
	}
	// --deep needs a token, so the two flags stop before any request.
	before := len(*seen)
	out = run(t, services, "check", "vincentsch/explainroo", "--deep", "--no-token", "--cache-dir", t.TempDir())
	if out.code != 1 || !strings.Contains(out.stderr, "cannot run with --no-token") || len(*seen) != before {
		t.Errorf("exit %d, %d requests, stderr %q", out.code, len(*seen)-before, out.stderr)
	}
}

func TestCompareCountsRequestsPerRepo(t *testing.T) {
	gh, _ := fakeGitHub(t, flagged, "vincentsch/explainroo")
	services := Services{GitHubURL: gh.URL}
	alone := run(t, services, "check", "vincentsch/explainroo", "--no-archive", "--json", "--cache-dir", t.TempDir())
	var single check.Result
	json.Unmarshal([]byte(alone.stdout), &single)

	out := run(t, services, "compare", flagged, "vincentsch/explainroo", "--no-archive", "--json", "--cache-dir", t.TempDir())
	var both comparison
	if err := json.Unmarshal([]byte(out.stdout), &both); err != nil || len(both.Repositories) != 2 {
		t.Fatalf("compare --json: %v\n%s", err, out.stdout)
	}
	if got := both.Repositories[1].Sources.GitHubRequests; got != single.Sources.GitHubRequests {
		t.Errorf("fzf used %d requests in compare and %d alone", got, single.Sources.GitHubRequests)
	}
}

func TestCtrlCDuringTheHistoryPrintsAPartialReport(t *testing.T) {
	// The first page of the history arrives, then the user presses Ctrl-C
	// while the second page loads. The check must print what it has, exit
	// 130 and keep the half check out of the cache.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.URL.Path == "/repos/me/tool":
			io.WriteString(w, `{"id":7,"full_name":"me/tool","html_url":"https://github.com/me/tool","created_at":"2024-01-01T00:00:00Z","stargazers_count":5000,"forks_count":300}`)
		case req.URL.Query().Get("page") == "2":
			cancel()
			<-req.Context().Done()
		default:
			w.Header().Set("Link", fmt.Sprintf(`<%s/repositories/7/stargazers/history?per_page=30&page=2>; rel="next"`, server.URL))
			var weeks []string
			sunday := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
			for i := 0; i < 30; i++ {
				weeks = append(weeks, fmt.Sprintf(`{"week":%d,"total":35,"days":[5,5,5,5,5,5,5]}`, sunday.AddDate(0, 0, -7*i).Unix()))
			}
			io.WriteString(w, "["+strings.Join(weeks, ",")+"]")
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	out := runContext(t, ctx, Services{GitHubURL: server.URL}, "check", "me/tool", "--no-archive", "--cache-dir", dir)
	if out.code != ExitInterrupted {
		t.Fatalf("exit %d, want %d: %s", out.code, ExitInterrupted, out.stderr)
	}
	if !strings.Contains(out.stdout, "Not enough data to judge. StarTrust read only part of the star history") ||
		!strings.Contains(out.stdout, "stopped (Ctrl-C)") || !strings.Contains(out.stderr, "partial and was not saved") {
		t.Errorf("stdout:\n%s\nstderr:\n%s", out.stdout, out.stderr)
	}
	if list := run(t, Services{}, "cache", "list", "--cache-dir", dir); !strings.Contains(list.stdout, "No saved checks") {
		t.Errorf("an interrupted check was saved:\n%s", list.stdout)
	}

	// Ctrl-C before anything was read: no report, but still exit 130.
	stopped, stop := context.WithCancel(context.Background())
	stop()
	out = runContext(t, stopped, Services{GitHubURL: server.URL}, "check", "me/tool", "--cache-dir", dir)
	if out.code != ExitInterrupted || !strings.Contains(out.stderr, "stopped (Ctrl-C)") {
		t.Errorf("exit %d, stderr %q", out.code, out.stderr)
	}
}

func TestRejectedTokenAndUsedUpLimitSayBoth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"message":"Bad credentials"}`)
			return
		}
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(testNow.Add(30*time.Minute).Unix()))
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"message":"API rate limit exceeded"}`)
	}))
	defer server.Close()
	services := Services{GitHubURL: server.URL, FindToken: func(context.Context) github.Token { return github.Token{Value: "expired", Source: "gh"} }}
	out := run(t, services, "check", "junegunn/fzf", "--no-archive", "--cache-dir", t.TempDir())
	// The usual advice to set a token would be wrong: the user did.
	if out.code != 6 || !strings.Contains(out.stderr, "GitHub rejected the token of your GitHub CLI login") ||
		!strings.Contains(out.stderr, "Run `gh auth login` to refresh it") || strings.Contains(out.stderr, "set GITHUB_TOKEN or run") {
		t.Errorf("exit %d, stderr %q", out.code, out.stderr)
	}
}

func TestFlagMistakesStopBeforeAnyRequest(t *testing.T) {
	gh, seen := fakeGitHub(t, "junegunn/fzf")
	dir := t.TempDir()
	for _, args := range [][]string{
		{"check", "junegunn/fzf", "--markdown", "--json"},
		{"check", "junegunn/fzf", "--out", dir},
		{"check", "junegunn/fzf", "charmbracelet/glow"},
	} {
		out := run(t, Services{GitHubURL: gh.URL}, append(args, "--cache-dir", dir)...)
		if out.code != 1 {
			t.Errorf("%v: exit %d, stderr %q", args, out.code, out.stderr)
		}
	}
	if len(*seen) != 0 {
		t.Errorf("flag mistakes cost %d GitHub requests", len(*seen))
	}
	if out := run(t, Services{}, "check", "a/b", "c/d"); !strings.Contains(out.stderr, "use `startrust compare` for several") {
		t.Errorf("stderr %q", out.stderr)
	}
}

func TestDetailsFitIn80Columns(t *testing.T) {
	for _, name := range []string{flagged, "junegunn/fzf", "vincentsch/explainroo"} {
		gh, _ := fakeGitHub(t, name)
		services := Services{GitHubURL: gh.URL, ArchiveURL: fixtureArchive(t, name).URL}
		out := run(t, services, "check", name, "--details", "--cache-dir", t.TempDir())
		for _, line := range strings.Split(out.stdout, "\n") {
			if n := len([]rune(line)); n > 80 {
				t.Errorf("%s: line is %d columns wide: %q", name, n, line)
			}
		}
		if strings.Contains(out.stdout, "not_needed") || strings.Contains(out.stdout, "list listed") {
			t.Errorf("%s: internal words in the details:\n%s", name, out.stdout)
		}
	}
}

func TestReportRefusesChecksFromOlderBuilds(t *testing.T) {
	// A saved check without a headline was judged by older rules.
	path := filepath.Join(t.TempDir(), "old.json")
	os.WriteFile(path, []byte(`{"schema_version":1,"repository":"a/b","risk":"low","verdict":"No unusual pattern."}`), 0o644)
	out := run(t, Services{}, "report", path)
	if out.code != 1 || !strings.Contains(out.stderr, "incompatible version") {
		t.Errorf("exit %d, stderr %q", out.code, out.stderr)
	}
}
