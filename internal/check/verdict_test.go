package check

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/startrust/internal/archive"
	"github.com/vincentsch/startrust/internal/stars"
)

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// quietThenSpike is a repo that got a star every other day for 200 days,
// then 500 stars in one day.
func quietThenSpike() (stars.Series, stars.Analysis) {
	daily := make([]int, 300)
	for i := range daily[:200] {
		daily[i] = i % 2
	}
	daily[200] = 500
	s := stars.Series{Start: day("2025-01-01"), Daily: daily}
	return s, stars.Analyze(s)
}

func cohort(d time.Time, checked, oneOff int) *archive.Cohort {
	return &archive.Cohort{Day: d, GitHubStars: checked, Archived: checked, Checked: checked, OneOff: oneOff}
}

func TestUnusualSpikeWithoutAccountDataIsMedium(t *testing.T) {
	s, a := quietThenSpike()
	r := &Result{Days: BiggestDays(s, a, BigDays)}
	Decide(r, a)
	if r.Risk != RiskMedium || r.Verdict != "One unusual spike, see below." {
		t.Errorf("risk %s, verdict %q", r.Risk, r.Verdict)
	}
	if !strings.Contains(strings.Join(r.Findings, "\n"), "Timing alone cannot tell") {
		t.Errorf("a timing-only verdict must say what timing cannot show: %v", r.Findings)
	}
}

func TestManyOneOffAccountsMakeItHigh(t *testing.T) {
	s, a := quietThenSpike()
	r := &Result{Days: BiggestDays(s, a, BigDays)}
	r.Days[0].Accounts = cohort(r.Days[0].Day, 400, 240)
	Decide(r, a)
	want := "Unusual star pattern. On 2025-07-20, 60% of the accounts that starred had no other public stars recorded in GH Archive within 30 days. " +
		"Bought or traded stars often look like this, so look closer before you trust the star count."
	if r.Risk != RiskHigh || r.Verdict != want {
		t.Errorf("risk %s, verdict %q", r.Risk, r.Verdict)
	}
}

func TestOneOffAccountsOnAnUnusualSpikeMakeItHigh(t *testing.T) {
	// 41% alone would be medium. On an unusual spike, timing and accounts
	// agree, and that makes it high.
	s, a := quietThenSpike()
	r := &Result{Days: BiggestDays(s, a, BigDays)}
	r.Days[0].Accounts = cohort(r.Days[0].Day, 400, 164)
	Decide(r, a)
	if r.Risk != RiskHigh || !strings.HasPrefix(r.Verdict, "Unusual star pattern. On 2025-07-20, 41% of the accounts") {
		t.Errorf("risk %s, verdict %q", r.Risk, r.Verdict)
	}
}

func TestSmallSampleCannotMakeItHigh(t *testing.T) {
	// Nine of thirty accounts is too small a sample to call high, even on
	// an unusual spike.
	s, a := quietThenSpike()
	r := &Result{Days: BiggestDays(s, a, BigDays)}
	r.Days[0].Accounts = cohort(r.Days[0].Day, 30, 9)
	Decide(r, a)
	if r.Risk != RiskMedium {
		t.Errorf("risk %s, verdict %q", r.Risk, r.Verdict)
	}
	r.Days[0].Accounts = cohort(r.Days[0].Day, 30, 27)
	Decide(r, a)
	if r.Risk != RiskMedium {
		t.Errorf("27 of 30: risk %s, verdict %q", r.Risk, r.Verdict)
	}
}

func TestViralLaunchWithManyOneOffAccountsIsMedium(t *testing.T) {
	// Repos that go viral with a wide audience draw people who rarely star
	// anything. A launch day with 38% one-off accounts is worth a look,
	// not a high risk.
	daily := make([]int, 300)
	daily[2], daily[3], daily[4] = 9000, 7000, 5000
	for i := 5; i < 300; i++ {
		daily[i] = 200
	}
	s := stars.Series{Start: day("2019-03-26"), Daily: daily}
	a := stars.Analyze(s)
	r := &Result{Days: BiggestDays(s, a, BigDays)}
	r.Days[0].Accounts = cohort(r.Days[0].Day, 1000, 377)
	Decide(r, a)
	want := "More one-off accounts than usual. On 2019-03-28, 38% of the accounts that starred had no other public stars recorded in GH Archive within 30 days. See below."
	if r.Risk != RiskMedium || r.Verdict != want {
		t.Errorf("risk %s, verdict %q", r.Risk, r.Verdict)
	}
}

func TestNormalAccountsClearTheSpike(t *testing.T) {
	s, a := quietThenSpike()
	r := &Result{Days: BiggestDays(s, a, BigDays)}
	r.Days[0].Accounts = cohort(r.Days[0].Day, 400, 12)
	r.BiggestDay = &r.Days[0]
	Decide(r, a)
	// The verdict says why the unusual spike does not count, instead of
	// calling it nothing next to a 500x day.
	want := "No unusual pattern. The accounts behind the unusual spike on 2025-07-20 look like most stargazers (3% one-off)."
	if r.Risk != RiskLow || r.Verdict != want || r.AccountsChecked != "all" || r.Headline != "no unusual pattern" {
		t.Errorf("risk %s, accounts checked %s, headline %q, verdict %q", r.Risk, r.AccountsChecked, r.Headline, r.Verdict)
	}
	if !strings.Contains(r.Findings[0], "The accounts behind it look like most stargazers (3% one-off).") {
		t.Errorf("the spike finding should say the accounts cleared it: %q", r.Findings[0])
	}
}

func TestElevatedAccountsAreWorthALook(t *testing.T) {
	s, a := quietThenSpike()
	r := &Result{Days: BiggestDays(s, a, BigDays)}
	r.Days[0].Accounts = cohort(r.Days[0].Day, 80, 24)
	Decide(r, a)
	// A share of 80 accounts comes with its sample size.
	want := "More one-off accounts than usual. On 2025-07-20, 24 of the 80 accounts that starred (30%) had no other public stars recorded in GH Archive within 30 days. See below."
	if r.Risk != RiskMedium || r.Verdict != want || r.Headline != "more one-off accounts" {
		t.Errorf("risk %s, headline %q, verdict %q", r.Risk, r.Headline, r.Verdict)
	}
}

func TestThinArchiveDataIsIgnoredAndExplained(t *testing.T) {
	s, a := quietThenSpike()
	r := &Result{Days: BiggestDays(s, a, BigDays)}
	r.Days[0].Accounts = &archive.Cohort{Day: r.Days[0].Day, GitHubStars: 500, Archived: 5, Checked: 5, OneOff: 5, Problem: "too few"}
	Decide(r, a)
	if r.Risk != RiskMedium {
		t.Errorf("risk %s: five archived accounts must not decide anything", r.Risk)
	}
	if !strings.Contains(strings.Join(r.Limits, "\n"), "too few records") {
		t.Errorf("limits should say why the accounts were not checked: %v", r.Limits)
	}
}

// launchOnly is a repo that launched on its third day and grew slowly
// after, checked on the given number of days after the launch.
func launchOnly(launch string, days int) (stars.Series, stars.Analysis) {
	daily := make([]int, days+3)
	daily[2] = 261
	for i := 3; i < len(daily); i++ {
		daily[i] = 10
	}
	s := stars.Series{Start: day(launch).AddDate(0, 0, -2), Daily: daily}
	return s, stars.Analyze(s)
}

func TestYoungRepoWithoutAccountDataIsTooNewToJudge(t *testing.T) {
	// GH Archive had 5 of 261 stargazers. Timing cannot judge a launch, so
	// nothing was judged, and the result must not read as a pass.
	// The verdict still gives the reader the repo's forks and issues.
	s, a := launchOnly("2026-10-01", 22)
	issues := 40
	r := &Result{Days: BiggestDays(s, a, BigDays), Sources: Sources{Archive: "used"}, Forks: 45, IssuesAndPRs: &issues,
		CreatedAt: day("2026-09-29"), CheckedAt: day("2026-10-26")}
	r.Days[0].Accounts = &archive.Cohort{Day: r.Days[0].Day, GitHubStars: 261, Archived: 5, Problem: "too few"}
	r.BiggestDay = &r.Days[0]
	Decide(r, a)
	want := "Too new to judge. The repo is 25 days old, and timing alone cannot tell a launch from a star campaign. " +
		"GH Archive has too few records of the busiest day to check the accounts. " +
		"So far it has 45 forks and 40 issues and pull requests, see below."
	if r.Risk != RiskUnknown || r.Verdict != want || r.Headline != "too new to judge" || r.AccountsChecked != "none" {
		t.Errorf("risk %s, headline %q, accounts checked %s, verdict %q", r.Risk, r.Headline, r.AccountsChecked, r.Verdict)
	}
}

func TestOldLaunchWithoutAccountDataSaysWhatWasNotChecked(t *testing.T) {
	// The same launch a year later: most stars came after it, steadily, so
	// the timing says something. The verdict still says the launch day's
	// accounts were not checked.
	s, a := launchOnly("2025-10-01", 400)
	r := &Result{Days: BiggestDays(s, a, BigDays), Sources: Sources{Archive: "used"}}
	r.Days[0].Accounts = &archive.Cohort{Day: r.Days[0].Day, GitHubStars: 261, Archived: 5, Problem: "too few"}
	r.BiggestDay = &r.Days[0]
	Decide(r, a)
	want := "No unusual pattern in the star timing. The accounts behind the launch day could not be checked."
	if r.Risk != RiskLow || r.Verdict != want || r.Headline != "no unusual timing" {
		t.Errorf("risk %s, headline %q, verdict %q", r.Risk, r.Headline, r.Verdict)
	}
}

func TestMostStarsFromAnUncheckedLaunchIsUnknown(t *testing.T) {
	// An old repo that got almost all its stars in its launch week. With
	// --no-archive, nothing judged those stars.
	daily := make([]int, 600)
	daily[10], daily[11], daily[12] = 3000, 2000, 1500
	daily[300] = 60
	s := stars.Series{Start: day("2023-01-01"), Daily: daily}
	a := stars.Analyze(s)
	// GitHub's search did not answer, so only the forks are known.
	r := &Result{Days: BiggestDays(s, a, BigDays), Sources: Sources{Archive: "off"}, Forks: 310,
		CreatedAt: day("2023-01-01"), CheckedAt: day("2024-08-23")}
	r.BiggestDay = &r.Days[0]
	Decide(r, a)
	want := "Not enough data to judge. Most stars arrived with the launch, and timing alone cannot tell a launch from a star campaign. " +
		"The accounts were not checked (--no-archive). It has 310 forks, see below."
	if r.Risk != RiskUnknown || r.Verdict != want || r.Headline != "not enough data" {
		t.Errorf("risk %s, headline %q, verdict %q", r.Risk, r.Headline, r.Verdict)
	}
}

func TestRecentLaunchIsProvisional(t *testing.T) {
	// The day was yesterday: the 30 days after it have not happened, so
	// 60% one-off accounts must not count.
	s, a := launchOnly("2026-10-25", 22)
	r := &Result{Days: BiggestDays(s, a, BigDays), Sources: Sources{Archive: "used"}}
	c := cohort(r.Days[0].Day, 100, 60)
	c.Provisional = true
	r.Days[0].Accounts = c
	r.BiggestDay = &r.Days[0]
	Decide(r, a)
	if r.Risk != RiskUnknown || !strings.Contains(r.Verdict, "The accounts behind the busiest day can be checked after 2026-11-26.") {
		t.Errorf("risk %s, verdict %q", r.Risk, r.Verdict)
	}
	if limits := strings.Join(r.Limits, "\n"); !strings.Contains(limits, "provisional") || !strings.Contains(limits, "after 2026-11-26") {
		t.Errorf("limits should say when the check counts: %v", r.Limits)
	}
	if r.Days[0].Judged || r.BiggestDay.Judged {
		t.Error("a provisional day is not judged")
	}
}

func TestEveryDaySaysWhetherItWasJudged(t *testing.T) {
	// The spike's accounts were checked; the quiet days were not. Each day
	// in the JSON still has judged and accounts, so nothing is missing.
	s, _ := quietThenSpike()
	s.Daily[250] = 60
	a := stars.Analyze(s)
	r := &Result{Days: BiggestDays(s, a, BigDays)}
	r.Days[0].Accounts = cohort(r.Days[0].Day, 400, 20)
	biggest := r.Days[0]
	r.BiggestDay = &biggest
	Decide(r, a)
	if len(r.Days) < 2 || !r.Days[0].Judged || !r.BiggestDay.Judged {
		t.Fatalf("the checked day must be judged: %+v", r.Days)
	}
	data, err := json.Marshal(r.Days)
	if err != nil {
		t.Fatal(err)
	}
	var days []map[string]any
	json.Unmarshal(data, &days)
	for i, d := range days {
		judged, hasJudged := d["judged"]
		accounts, hasAccounts := d["accounts"]
		if !hasJudged || !hasAccounts {
			t.Errorf("day %d has no judged or accounts field: %s", i+1, data)
		}
		if i > 0 && (judged != false || accounts != nil) {
			t.Errorf("day %d was not checked: judged %v, accounts %v", i+1, judged, accounts)
		}
	}
	if accounts, _ := days[0]["accounts"].(map[string]any); accounts == nil || accounts["judged"] != nil {
		t.Errorf("judged belongs to the day, not to its accounts: %s", data)
	}
}

func TestClearedSpikeStillCountsInTheVerdict(t *testing.T) {
	// Two unusual spikes; the accounts cleared the first. The verdict, the
	// headline and the findings must all speak of two spikes.
	daily := make([]int, 500)
	for i := range daily {
		daily[i] = i % 2
	}
	daily[200], daily[400] = 500, 400
	s := stars.Series{Start: day("2024-01-01"), Daily: daily}
	a := stars.Analyze(s)
	r := &Result{Days: BiggestDays(s, a, BigDays)}
	r.Days[0].Accounts = cohort(r.Days[0].Day, 400, 12)
	Decide(r, a)
	if r.Risk != RiskMedium || r.Verdict != "2 unusual spikes, see below." || r.Headline != "2 unusual spikes" {
		t.Errorf("risk %s, headline %q, verdict %q", r.Risk, r.Headline, r.Verdict)
	}
	if !strings.Contains(r.Findings[0], "look like most stargazers") || strings.Contains(r.Findings[1], "look like most stargazers") {
		t.Errorf("findings = %q", r.Findings)
	}
}

func TestPartialHistoryIsNotReassuring(t *testing.T) {
	daily := make([]int, 200)
	for i := range daily {
		daily[i] = 3
	}
	s := stars.Series{Start: day("2026-04-01"), Daily: daily}
	a := stars.Analyze(s)
	r := &Result{Stars: 70000, Days: BiggestDays(s, a, BigDays), HistoryPartial: true, HistoryFrom: s.Start, HistoryTo: s.End(),
		Sources: Sources{HistoryProblem: "stopped after 2 GitHub requests (--max-requests)"}}
	Decide(r, a)
	want := "Not enough data to judge. StarTrust read only part of the star history (600 of 70,000 stars, from 2026-04-01 on), and nothing unusual shows in that part."
	if r.Risk != RiskUnknown || r.Verdict != want {
		t.Errorf("risk %s, verdict %q", r.Risk, r.Verdict)
	}
}

func TestProfilesNeverChangeTheRisk(t *testing.T) {
	s, a := quietThenSpike()
	r := &Result{Days: BiggestDays(s, a, BigDays)}
	r.Days[0].Accounts = cohort(r.Days[0].Day, 400, 12)
	r.Days[0].Profiles = &Profiles{Accounts: 500, New: 400, NewOrEmpty: 400}
	Decide(r, a)
	if r.Risk != RiskLow {
		t.Errorf("risk %s: --deep profile counts are evidence only", r.Risk)
	}
}

func TestNoStars(t *testing.T) {
	r := &Result{}
	Decide(r, stars.Analysis{})
	if r.Risk != RiskUnknown || !strings.HasPrefix(r.Verdict, "No stars yet") || r.Headline != "no stars yet" {
		t.Errorf("risk %s, headline %q, verdict %q", r.Risk, r.Headline, r.Verdict)
	}
	// Limits about stars that are not there would only be noise.
	if len(r.Limits) != 0 {
		t.Errorf("limits = %q", r.Limits)
	}
}

func TestSteadyGrowth(t *testing.T) {
	daily := make([]int, 400)
	for i := range daily {
		daily[i] = 10 + i%5
	}
	daily[300] = 60
	s := stars.Series{Start: day("2024-01-01"), Daily: daily}
	a := stars.Analyze(s)
	r := &Result{Days: BiggestDays(s, a, BigDays), Sources: Sources{Archive: "used"}}
	r.BiggestDay = &r.Days[0]
	r.Days[0].Accounts = cohort(r.Days[0].Day, 60, 1)
	Decide(r, a)
	if r.Risk != RiskLow || r.Verdict != "No unusual pattern. The stars arrived steadily, and the biggest day had 60 stars, 5x a normal day." {
		t.Errorf("risk %s verdict %q", r.Risk, r.Verdict)
	}
	// Without the account check, the same history only speaks for the
	// timing.
	r = &Result{Days: BiggestDays(s, a, BigDays), Sources: Sources{Archive: "off"}}
	r.BiggestDay = &r.Days[0]
	Decide(r, a)
	if r.Risk != RiskLow || r.Verdict != "No unusual pattern in the star timing. The accounts behind the busiest days were not checked." {
		t.Errorf("--no-archive: risk %s verdict %q", r.Risk, r.Verdict)
	}
}

func TestSmallRepo(t *testing.T) {
	daily := make([]int, 400)
	for i := 0; i < 400; i += 10 {
		daily[i] = 1
	}
	s := stars.Series{Start: day("2024-01-01"), Daily: daily}
	a := stars.Analyze(s)
	r := &Result{Days: BiggestDays(s, a, BigDays)}
	Decide(r, a)
	if r.Risk != RiskLow || r.AccountsChecked != "none" || !strings.Contains(r.Verdict, "too few stars to check the accounts") {
		t.Errorf("risk %s verdict %q", r.Risk, r.Verdict)
	}
}

func TestBiggestDaysAreSpreadOut(t *testing.T) {
	daily := make([]int, 60)
	daily[10], daily[11], daily[12] = 100, 90, 80
	daily[40] = 70
	s := stars.Series{Start: day("2025-01-01"), Daily: daily}
	days := BiggestDays(s, stars.Analyze(s), BigDays)
	if len(days) != 2 || days[0].Stars != 100 || days[1].Stars != 70 {
		t.Errorf("days = %+v", days)
	}
}

func TestRiskRank(t *testing.T) {
	if RiskRank(RiskHigh) <= RiskRank(RiskMedium) || RiskRank(RiskMedium) <= RiskRank(RiskLow) || RiskRank("bogus") != -1 {
		t.Error("risk order is wrong")
	}
}

func TestArchiveLimitsSayWhatIsMissing(t *testing.T) {
	daily := make([]int, 3000)
	for i := range daily {
		daily[i] = 10
	}
	daily[100], daily[1500], daily[2500] = 584, 694, 900
	s := stars.Series{Start: day("2010-07-23"), Daily: daily}
	a := stars.Analyze(s)
	r := &Result{Repository: "me/tool", Stars: s.Total(), Days: BiggestDays(s, a, BigDays), Sources: Sources{Archive: "used"}}
	today := day("2026-10-26")
	for i := range r.Days {
		c := &archive.Cohort{Day: r.Days[i].Day, GitHubStars: r.Days[i].Stars, WindowRecorded: 1}
		// Day 1500 has no record at all: the repo probably had another
		// name. Day 2500 has plenty on the day, but GH Archive missed
		// most stars around it.
		if r.Days[i].Day.Equal(s.Day(2500)) {
			c.Archived, c.Checked, c.OneOff, c.WindowRecorded = 880, 880, 600, 0.3
		}
		c.Judge(today)
		r.Days[i].Accounts = c
	}
	r.BiggestDay = &r.Days[0]
	Decide(r, a)
	limits := strings.Join(r.Limits, "\n")
	if !r.RenameSuspected {
		t.Error("a busy day without any record must make a rename suspected")
	}
	for _, want := range []string{
		"GH Archive starts in February 2011, so it has no record of 2010-10-31.",
		"GH Archive has no stars at all for me/tool on 2014-08-31. It files stars under the name a repo had at the time, " +
			"so the repo may have had another name then. If it did, run `startrust check me/tool --former-name OLD-OWNER/OLD-NAME` with that name.",
		"GH Archive missed many stars across GitHub in the 30 days before and after 2017-05-27. " +
			"The accounts from that day could look one-off only because their other stars are missing, so StarTrust did not judge it.",
	} {
		if !strings.Contains(limits, want) {
			t.Errorf("limits are missing %q:\n%s", want, limits)
		}
	}
}

func TestRenameHintNamesTheCommandToRun(t *testing.T) {
	// withastro/astro was snowpackjs/astro at its launch. A check that
	// already looked up one former name and still found nothing on a day
	// says how to add another one, with the names it already has.
	s, a := quietThenSpike()
	r := &Result{Repository: "withastro/astro", FormerNames: []string{"snowpackjs/astro"}, Stars: s.Total(),
		Days: BiggestDays(s, a, BigDays), Sources: Sources{Archive: "used"}}
	r.Days[0].Accounts = &archive.Cohort{Day: r.Days[0].Day, GitHubStars: r.Days[0].Stars, WindowRecorded: 1}
	r.Days[0].Accounts.Judge(day("2026-10-26"))
	Decide(r, a)
	want := "GH Archive has no stars at all for withastro/astro or snowpackjs/astro on 2025-07-20. " +
		"It files stars under the name a repo had at the time, so the repo may have had another name then. If it did, run " +
		"`startrust check withastro/astro --former-name snowpackjs/astro --former-name OLD-OWNER/OLD-NAME` with that name."
	if !r.RenameSuspected || !strings.Contains(strings.Join(r.Limits, "\n"), want) {
		t.Errorf("rename suspected %v, limits:\n%s", r.RenameSuspected, strings.Join(r.Limits, "\n"))
	}
}

func TestNoRenameHintWhereGHArchiveIsNearlyEmpty(t *testing.T) {
	// Since June 2026 GH Archive misses almost every star, so a busy day
	// without any record says nothing about the repo's name.
	daily := make([]int, 300)
	daily[200] = 500
	s := stars.Series{Start: day("2026-01-01"), Daily: daily}
	a := stars.Analyze(s)
	r := &Result{Repository: "me/new", Stars: 500, Days: BiggestDays(s, a, BigDays), Sources: Sources{Archive: "used"}}
	r.Days[0].Accounts = &archive.Cohort{Day: r.Days[0].Day, GitHubStars: 500, WindowRecorded: 1}
	r.Days[0].Accounts.Judge(day("2026-10-26"))
	Decide(r, a)
	limits := strings.Join(r.Limits, "\n")
	if r.RenameSuspected || strings.Contains(limits, "--former-name") || !strings.Contains(limits, "too few records") {
		t.Errorf("day %s: rename suspected %v, limits:\n%s", date(r.Days[0].Day), r.RenameSuspected, limits)
	}
}

func TestNoRatioWithoutANormalDay(t *testing.T) {
	// A normal day of 0 gives no ratio, and the verdict leaves it out.
	quiet := Day{Day: day("2025-02-09"), Stars: 20}
	if got := timingSummary(&quiet); strings.Contains(got, "normal day") {
		t.Errorf("summary %q", got)
	}
	if ratio := timesNormal(20, 0); ratio != nil {
		t.Errorf("ratio %v for a normal day of 0", *ratio)
	}
	if ratio := timesNormal(20, 0.5); ratio == nil || *ratio != 20 {
		t.Errorf("a normal day under 1 star counts as 1: %v", ratio)
	}
}

func TestRatioIsRoundedOnce(t *testing.T) {
	// The busy-day list stores 30.5 for a ratio of 30.47; the spike list
	// keeps 30.47. Both must print the same.
	if RatioText(30.47) != RatioText(round1(30.47)) || RatioText(30.47) != "31" {
		t.Errorf("%s and %s", RatioText(30.47), RatioText(round1(30.47)))
	}
}

func TestActivitySentenceForAnOlderRepo(t *testing.T) {
	// A repo older than 90 days: the issue count covers the last 90 days,
	// and the verdict says so.
	one, many := 1, 12
	r := &Result{Forks: 8534, IssuesAndPRs: &many, CreatedAt: day("2024-03-17"), CheckedAt: day("2026-10-26")}
	if got := activitySentence(r); got != "It has 8,534 forks, and 12 issues and pull requests were opened in the last 90 days, see below." {
		t.Errorf("got %q", got)
	}
	r.Forks, r.IssuesAndPRs = 1, &one
	if got := activitySentence(r); got != "It has 1 fork, and 1 issue or pull request was opened in the last 90 days, see below." {
		t.Errorf("got %q", got)
	}
}

func TestForksPer1000Text(t *testing.T) {
	for _, c := range []struct {
		stars, forks int
		want         string
	}{
		{83796, 4698, "56"}, {1600, 5, "3.1"}, {3000, 8, "2.7"}, {20112, 1971, "98"}, {5000, 0, "0"}, {2000, 2, "1"},
	} {
		r := Result{Stars: c.stars, Forks: c.forks}
		if got := r.ForksPer1000Text(); got != c.want {
			t.Errorf("%d forks for %d stars: %q, want %q", c.forks, c.stars, got, c.want)
		}
	}
}

// TestReferenceNoteMatchesTheComparison replays every organic snapshot of
// the forks comparison in docs/how-it-works.md: 182 repos today, 70 of them
// 30 days after their busiest day, and 9 launches 1, 3 and 7 days after
// their busiest day. The note under every report's activity quotes the
// number of repos and the fewest forks per 1,000 stars among them.
func TestReferenceNoteMatchesTheComparison(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "organic-forks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var snapshots []struct {
		Repo, When, Created, Checked string
		Stars, Forks                 int
	}
	if err := json.Unmarshal(raw, &snapshots); err != nil {
		t.Fatal(err)
	}
	today, lowest := 0, 1000.0
	for _, s := range snapshots {
		r := &Result{Stars: s.Stars, Forks: s.Forks}
		if s.When == "today" {
			today++
		}
		if r.Stars >= ActivityMinStars {
			lowest = min(lowest, r.ForksPer1000())
		}
	}
	if today != OrganicRepos {
		t.Errorf("%d organic repos in the data, OrganicRepos says %d", today, OrganicRepos)
	}
	// The reference in every report says no organic repo had fewer.
	if lowest < OrganicMinForks {
		t.Errorf("an organic repo had %.1f forks per 1,000 stars, under OrganicMinForks", lowest)
	}
}

func TestForksNeverChangeTheRisk(t *testing.T) {
	// StarTrust tried to raise the risk of repos with very few forks and
	// dropped it: honest packages that tools such as composer thanks star
	// automatically have almost no forks. Forks are context only.
	checked := day("2026-10-26")
	// A young launch nobody could judge: 5 forks for 1,600 stars.
	s, a := launchOnly("2026-08-01", 80)
	r := &Result{Days: BiggestDays(s, a, BigDays), Sources: Sources{Archive: "off"}, Stars: 1600, Forks: 5,
		CreatedAt: day("2026-07-30"), CheckedAt: checked}
	Decide(r, a)
	if r.Risk != RiskUnknown || r.Headline != "too new to judge" || !strings.HasSuffix(r.Verdict, "It has 5 forks, see below.") {
		t.Errorf("risk %s, headline %q, verdict %q", r.Risk, r.Headline, r.Verdict)
	}
	all := strings.Join(append(append([]string{}, r.Findings...), r.Limits...), "\n")
	if strings.Contains(strings.ToLower(all), "few forks") {
		t.Errorf("no finding or limit should be about few forks:\n%s", all)
	}

	// A high risk stays high, and the forks add nothing.
	s, a = launchOnly("2025-01-01", 400)
	r = &Result{Days: BiggestDays(s, a, BigDays), Sources: Sources{Archive: "used"}, Stars: 20000, Forks: 10,
		CreatedAt: day("2024-12-30"), CheckedAt: checked}
	r.Days[0].Accounts = cohort(r.Days[0].Day, 1000, 900)
	Decide(r, a)
	if r.Risk != RiskHigh || strings.Contains(strings.Join(r.Findings, "\n"), "forks") {
		t.Errorf("risk %s, findings %q", r.Risk, r.Findings)
	}
}
