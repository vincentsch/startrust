package check

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/startrust/internal/archive"
	"github.com/vincentsch/startrust/internal/github"
	"github.com/vincentsch/startrust/internal/stars"
)

// calibrationDay is when the calibration data was captured.
var calibrationDay = time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)

// fixture is a real star history from GitHub, with the GH Archive answers
// for its busiest days. Repos that research flagged are stored under made-up
// names such as flagged_a/repo. GitHub user names cannot contain an
// underscore, so these names cannot point at anyone. Their dates are moved
// back 61 weeks, and every count is scaled down by a fixed factor between
// 0.85 and 0.95 per repo (stars, forks, watchers, issues and the GH Archive
// numbers), so the numbers cannot be looked up either. Spikes, bursts,
// shares and ratios stay the same.
type fixture struct {
	Repo struct {
		FullName  string    `json:"full_name"`
		CreatedAt time.Time `json:"created_at"`
		Stars     int       `json:"stargazers_count"`
		Forks     int       `json:"forks_count"`
	} `json:"repo"`
	Accounts []archive.Cohort `json:"accounts"`
	Weeks    []github.Week    `json:"weeks"`
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

// replay runs the decision on a fixture the way a check on the capture day
// would have, with the saved GH Archive answers. Without archive, it judges
// the timing alone.
func replay(t *testing.T, f fixture, withArchive bool) (*Result, stars.Analysis) {
	t.Helper()
	s := stars.FromWeeks(f.Weeks, f.Repo.CreatedAt, calibrationDay)
	a := stars.Analyze(s)
	r := &Result{Repository: f.Repo.FullName, Stars: f.Repo.Stars, Forks: f.Repo.Forks, Days: BiggestDays(s, a, BigDays),
		Sources: Sources{Archive: "off"}, HistoryFrom: s.Start, HistoryTo: s.End(),
		CreatedAt: f.Repo.CreatedAt, CheckedAt: calibrationDay}
	if withArchive {
		r.Sources.Archive = "used"
		for _, i := range AccountTargets(r.Days, a.Spikes) {
			day := &r.Days[i]
			for _, c := range f.Accounts {
				if c.Day.Equal(day.Day) {
					c.Judge(calibrationDay)
					day.Accounts = &c
				}
			}
			if day.Accounts == nil {
				t.Errorf("no saved account check for %s; capture the fixture again", date(day.Day))
			}
		}
	}
	if len(r.Days) > 0 {
		r.BiggestDay = &r.Days[0]
	}
	Decide(r, a)
	return r, a
}

// TestCalibration replays every repo of the calibration set. The table in
// docs/how-it-works.md shows the same results; update both together.
func TestCalibration(t *testing.T) {
	const (
		unknown, low, medium, high = RiskUnknown, RiskLow, RiskMedium, RiskHigh
	)
	cases := []struct {
		name    string
		flagged bool // flagged by research or press reports
		unusual int
		burst   bool
		risk    string // with the account check
		// timing is the risk with --no-archive. It is unknown when most
		// stars came with the launch, which timing alone cannot judge.
		timing string
	}{
		// Flagged by the StarScout study or press reports.
		{"flagged_a/repo", true, 1, true, high, medium},
		{"flagged_b/repo", true, 1, true, high, medium},
		{"flagged_c/repo", true, 1, true, high, medium},
		{"flagged_d/repo", true, 1, true, high, medium},
		{"flagged_e/repo", true, 0, true, high, medium},
		{"flagged_f/repo", true, 0, false, high, low},
		{"flagged_g/repo", true, 0, true, high, medium},
		{"flagged_h/repo", true, 0, false, high, low},
		{"flagged_i/repo", true, 0, false, high, unknown},
		{"flagged_j/repo", true, 0, true, high, medium},
		// Flagged for accounts that star in lockstep. Those accounts star
		// many repos, so the one-off check cannot see them. flagged_m is
		// high because of days months after the flagged ones.
		{"flagged_k/repo", true, 0, false, low, unknown},
		{"flagged_l/repo", true, 0, false, low, unknown},
		{"flagged_m/repo", true, 1, false, high, medium},
		{"flagged_n/repo", true, 0, false, low, low},
		// Flagged for a smaller share of its stars.
		{"flagged_o/repo", true, 0, false, medium, low},

		// Developer tools and launches.
		{"junegunn/fzf", false, 0, false, low, low},
		{"charmbracelet/glow", false, 0, false, low, low},
		{"openai/codex", false, 0, false, low, low},
		{"astral-sh/uv", false, 0, false, low, low},
		{"jesseduffield/lazygit", false, 0, false, low, low},
		{"yoheinakajima/babyagi", false, 0, false, low, low},
		{"vincentsch/explainroo", false, 0, false, unknown, unknown},
		{"openai/whisper", false, 0, false, low, low},
		{"twitter/the-algorithm", false, 1, false, low, medium},
		{"Significant-Gravitas/AutoGPT", false, 1, false, low, medium},
		{"deepseek-ai/DeepSeek-R1", false, 0, false, low, low},
		{"xai-org/grok-1", false, 0, false, low, unknown},
		// Went viral long after they were created. That makes an unusual
		// spike, which the account check clears.
		{"hacksider/Deep-Live-Cam", false, 1, false, low, medium},
		{"Stirling-Tools/Stirling-PDF", false, 0, false, low, low},
		{"ytdl-org/youtube-dl", false, 1, false, low, medium},
		// Lists and courses with a wide audience.
		{"sindresorhus/awesome", false, 0, false, low, low},
		{"vinta/awesome-python", false, 0, false, low, low},
		{"public-apis/public-apis", false, 0, false, low, low},
		{"EbookFoundation/free-programming-books", false, 0, false, medium, low},
		{"nilbuild/developer-roadmap", false, 0, false, low, low},
		{"jackfrued/Python-100-Days", false, 0, false, low, low},
		// Viral cause and data repos.
		{"996icu/996.ICU", false, 0, false, medium, low},
		{"kattgu7/Anti-996-License", false, 0, false, low, low},
		{"wuhan2020/wuhan2020", false, 0, false, low, unknown},
		{"CSSEGISandData/COVID-19", false, 0, false, low, low},
		{"owid/covid-19-data", false, 0, false, low, low},
		{"nytimes/covid-19-data", false, 0, false, low, low},
		// Small repos.
		{"rxi/microui", false, 0, false, low, low},
		{"antonmedv/countdown", false, 0, false, low, low},
		{"nalgeon/sqlean", false, 1, false, low, medium},
		{"zserge/jsmn", false, 0, false, low, low},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := loadFixture(t, c.name)
			r, a := replay(t, f, true)
			timing, _ := replay(t, f, false)
			var shares []string
			for _, d := range r.Days {
				if d.Accounts != nil && d.Accounts.Usable() {
					shares = append(shares, fmt.Sprintf("%s %d%% of %d", date(d.Day), percent(d.Accounts.Share()), d.Accounts.Checked))
				}
			}
			t.Logf("%-40s %-6s %-6s unusual %d burst %v | %s | %s", c.name, r.Risk, timing.Risk, len(a.Unusual()), a.Burst != nil, strings.Join(shares, ", "), r.Verdict)
			// The summary and the history were read seconds apart, so a
			// star or two may differ.
			if diff := a.Total - f.Repo.Stars; diff < -2 || diff > 2 {
				t.Errorf("history has %d stars, repo summary %d", a.Total, f.Repo.Stars)
			}
			if got := len(a.Unusual()); got != c.unusual {
				t.Errorf("unusual spikes = %d, want %d", got, c.unusual)
			}
			if got := a.Burst != nil; got != c.burst {
				t.Errorf("burst = %v, want %v", got, c.burst)
			}
			if r.Risk != c.risk {
				t.Errorf("risk %s, want %s: %s", r.Risk, c.risk, r.Verdict)
			}
			if timing.Risk != c.timing {
				t.Errorf("timing alone: risk %s, want %s: %s", timing.Risk, c.timing, timing.Verdict)
			}
			if !c.flagged && r.Risk == RiskHigh {
				t.Errorf("an organic repo must never be high")
			}
		})
	}
}
