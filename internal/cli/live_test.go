package cli

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/vincentsch/startrust/internal/check"
	"github.com/vincentsch/startrust/internal/github"
)

// TestLive checks well-known repos against the real GitHub API and GH
// Archive. It catches changes on their side that fixtures cannot, such as
// the page links GitHub sends. Run it with `make live`; it uses
// GITHUB_TOKEN when set and about 25 GitHub requests for fzf.
//
// Set STARTRUST_LIVE_FLAGGED to owner/repo of a repo you expect to come out
// medium or high, to check that side too. The repo stays out of the code on
// purpose: StarTrust never names a third-party repo as suspect.
func TestLive(t *testing.T) {
	if os.Getenv("STARTRUST_LIVE") != "1" {
		t.Skip("set STARTRUST_LIVE=1 to talk to the real GitHub API")
	}
	services := Services{
		Getenv: os.Getenv,
		FindToken: func(context.Context) github.Token {
			if value := os.Getenv("GITHUB_TOKEN"); value != "" {
				return github.Token{Value: value, Source: "GITHUB_TOKEN"}
			}
			return github.Token{}
		},
	}
	cases := []struct {
		repo  string
		risks []string
	}{
		// Steady growth for 13 years.
		{"junegunn/fzf", []string{check.RiskLow}},
	}
	if flagged := os.Getenv("STARTRUST_LIVE_FLAGGED"); flagged != "" {
		// Medium when GH Archive is unavailable, high with it.
		cases = append(cases, struct {
			repo  string
			risks []string
		}{flagged, []string{check.RiskMedium, check.RiskHigh}})
	}
	for _, c := range cases {
		t.Run(c.repo, func(t *testing.T) {
			var stdout, stderr stringBuffer
			code := NewApp(services).Run([]string{"check", c.repo, "--json", "--details", "--cache-dir", t.TempDir()}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("exit %d: %s", code, stderr)
			}
			var r check.Result
			if err := json.Unmarshal([]byte(stdout.String()), &r); err != nil {
				t.Fatal(err)
			}
			if r.Sources.History != "complete" {
				t.Errorf("history %s: %s", r.Sources.History, r.Sources.HistoryProblem)
			}
			total := r.Details.History.Total()
			if diff := total - r.Stars; diff < -50 || diff > 50 {
				t.Errorf("history has %d stars, GitHub shows %d", total, r.Stars)
			}
			ok := false
			for _, risk := range c.risks {
				ok = ok || r.Risk == risk
			}
			if !ok {
				t.Errorf("risk %s, want one of %v: %s", r.Risk, c.risks, r.Verdict)
			}
			t.Logf("%s: %s (%d GitHub requests, %d archive queries, archive %s)", c.repo, r.Verdict,
				r.Sources.GitHubRequests, r.Sources.ArchiveQueries, r.Sources.Archive)
		})
	}
}

type stringBuffer struct{ data []byte }

func (b *stringBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}
func (b *stringBuffer) String() string { return string(b.data) }
