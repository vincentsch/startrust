package check

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/vincentsch/startrust/internal/archive"
	"github.com/vincentsch/startrust/internal/stars"
)

// Thresholds for the account checks. A one-off account is one with no other
// public star recorded in GH Archive in the 30 days before or after it
// starred. docs/how-it-works.md shows the calibration behind these numbers.
const (
	// StrongShare of one-off accounts on a day makes the risk high on its
	// own. Most days of repos flagged by research were above it. No organic
	// day in the calibration set went over 38%.
	StrongShare = 0.50
	// ElevatedShare of one-off accounts is more than organic developer
	// tools showed (up to 21%). Viral repos with a wide audience reached 38%,
	// so on its own this share only makes the risk medium. It makes the
	// risk high when the same day is part of an unusual spike or a burst,
	// because then two separate signals agree.
	ElevatedShare = 0.30
	// MinSample is the smallest number of checked accounts that can make the
	// risk high. With 30 accounts, a share can be off by about 15
	// percentage points.
	MinSample = 100

	// NewAccountDays is how young an account can be when it stars and
	// still count as new. Profile counts from --deep are shown as evidence
	// but do not change the risk, because there are not enough known cases
	// yet to say what share is normal.
	NewAccountDays = 30
	// MinProfiles is the smallest group of stargazers worth summing up.
	MinProfiles = 30

	// BigDays is how many of the biggest days a result lists.
	BigDays = 5
	// DaysApart stops the listed days from all landing in one spike.
	DaysApart = 7
)

// dayLevel says how worrying one day's account data is.
type dayLevel int

const (
	levelNone     dayLevel = iota // no account data that counts
	levelNormal                   // accounts look like most repos
	levelElevated                 // more one-off accounts than usual
	levelHigh                     // many one-off accounts in a big sample
)

// Counts reports whether the day's account check can change the risk: GH
// Archive had enough of its stargazers, and the 30 days after it are over.
func (d Day) Counts() bool {
	return d.Accounts != nil && d.Accounts.Judged()
}

func (d Day) level() dayLevel {
	if !d.Counts() {
		return levelNone
	}
	share := d.Accounts.Share()
	switch {
	case share >= StrongShare && d.Accounts.Checked >= MinSample:
		return levelHigh
	case share >= ElevatedShare:
		return levelElevated
	}
	return levelNormal
}

// BiggestDays picks up to n of the busiest days, at least DaysApart days
// apart, each with at least stars.SpikeMinStars stars. Unusual spikes are
// always included so the account check can look at them.
func BiggestDays(s stars.Series, a stars.Analysis, n int) []Day {
	order := make([]int, len(s.Daily))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return s.Daily[order[i]] > s.Daily[order[j]] })

	var picked []int
	tooClose := func(i int) bool {
		for _, p := range picked {
			if abs(p-i) < DaysApart {
				return true
			}
		}
		return false
	}
	for _, spike := range a.Unusual() {
		if i := s.Index(spike.Peak); i >= 0 && !tooClose(i) {
			picked = append(picked, i)
		}
	}
	for _, i := range order {
		if len(picked) >= n || s.Daily[i] < stars.SpikeMinStars {
			break
		}
		if !tooClose(i) {
			picked = append(picked, i)
		}
	}
	sort.SliceStable(picked, func(i, j int) bool { return s.Daily[picked[i]] > s.Daily[picked[j]] })

	days := make([]Day, 0, len(picked))
	for _, i := range picked {
		normal := stars.Median(s.Daily[max(0, i-stars.BaselineDays):i])
		day := Day{Day: s.Day(i), Stars: s.Daily[i], NormalDay: normal, Ratio: round1(stars.Ratio(s.Daily[i], normal))}
		for _, spike := range a.Spikes {
			if !day.Day.Before(spike.Start) && !day.Day.After(spike.End) {
				switch {
				case spike.Unusual:
					day.Spike = "unusual"
				case spike.Launch:
					day.Spike = "launch"
				default:
					day.Spike = "spike"
				}
			}
		}
		days = append(days, day)
	}
	return days
}

// AccountTargets returns the positions of the days whose accounts get
// checked: up to ArchiveDays of the biggest days with at least
// ArchiveMinStars stars, plus the peak of every unusual spike.
func AccountTargets(days []Day, spikes []stars.Spike) []int {
	unusualPeak := map[time.Time]bool{}
	for _, spike := range spikes {
		if spike.Unusual {
			unusualPeak[spike.Peak] = true
		}
	}
	var targets []int
	for i, d := range days {
		if d.Stars >= ArchiveMinStars && (unusualPeak[d.Day] || len(targets) < ArchiveDays) {
			targets = append(targets, i)
		}
	}
	return targets
}

// Decide sets the risk, verdict, headline, findings and limits from the
// analysis and whatever account data the result holds.
func Decide(r *Result, a stars.Analysis) {
	r.Findings = []string{}
	r.Limits = []string{}
	r.UnusualSpikes = len(a.Unusual())
	r.Burst = a.Burst

	targets := AccountTargets(r.Days, a.Spikes)
	r.AccountsChecked = accountsChecked(r.Days, targets)
	for i := range r.Days {
		r.Days[i].Judged = r.Days[i].Counts()
	}
	if r.BiggestDay != nil {
		r.BiggestDay.Judged = r.BiggestDay.Counts()
	}

	if a.Total == 0 {
		r.Risk = RiskUnknown
		r.Headline = "no stars yet"
		r.Verdict = "No stars yet, so there is nothing to check."
		if r.Stars > 0 {
			r.Headline = "not enough data"
			r.Verdict = "GitHub returned no star history for this repo, so there is nothing to check."
		}
		r.Limits = append(r.Limits, baseLimits(r, targets)...)
		return
	}

	inBurst := func(d Day) bool {
		return a.Burst != nil && !d.Day.Before(a.Burst.Start) && !d.Day.After(a.Burst.End)
	}
	// A day with many one-off accounts counts as high on its own. A day
	// with more than usual counts as high only when its timing is also
	// unusual, because then two separate signals agree.
	var high, elevated []Day
	for _, d := range r.Days {
		switch d.level() {
		case levelHigh:
			high = append(high, d)
		case levelElevated:
			if (d.Spike == "unusual" || inBurst(d)) && d.Accounts.Checked >= MinSample {
				high = append(high, d)
			} else {
				elevated = append(elevated, d)
			}
		}
	}

	// Timing findings that account data can clear.
	unusual := a.Unusual()
	var cleared []Day
	for _, spike := range unusual {
		if d, ok := dayAt(r.Days, spike.Peak); ok && d.level() != levelNone {
			cleared = append(cleared, d)
		}
	}
	unclearedSpikes := len(unusual) - len(cleared)
	burstCleared := false
	if a.Burst != nil {
		// The burst is cleared when the busiest checked day inside it had
		// normal accounts.
		for _, d := range r.Days {
			if inBurst(d) && d.level() != levelNone {
				burstCleared = d.level() == levelNormal
				break
			}
		}
	}
	unclearedBurst := a.Burst != nil && !burstCleared

	blind := false
	switch {
	case len(high) > 0:
		r.Risk = RiskHigh
		r.Headline = "unusual pattern"
		r.Verdict = "Unusual star pattern. " + accountSentence(high) +
			". Bought or traded stars often look like this, so look closer before you trust the star count."
	case len(elevated) > 0:
		r.Risk = RiskMedium
		r.Headline = "more one-off accounts"
		r.Verdict = "More one-off accounts than usual. " + accountSentence(elevated) + ". See below."
	case unclearedSpikes > 0 || unclearedBurst:
		// The verdict counts every unusual spike, like the findings below
		// it. The findings say which ones the account check cleared.
		spikes := 0
		if unclearedSpikes > 0 {
			spikes = len(unusual)
		}
		r.Risk = RiskMedium
		r.Headline = timingHeadline(spikes)
		r.Verdict = timingVerdict(spikes, unclearedBurst)
	case r.HistoryPartial:
		// The missing part of the history may hold the very thing the check
		// looks for, so finding nothing in the rest proves little.
		r.Risk = RiskUnknown
		r.Headline = "not enough data"
		r.Verdict = fmt.Sprintf("Not enough data to judge. StarTrust read only part of the star history (%s of %s stars, from %s on), "+
			"and nothing unusual shows in that part.", Number(a.Total), Number(r.Stars), date(r.HistoryFrom))
	case r.AccountsChecked == "none" && timingBlind(a):
		// Timing cannot judge a launch, and a young repo has no quiet
		// period to compare with. Without an account check there is
		// nothing left to judge by, which is not the same as nothing
		// unusual.
		r.Risk = RiskUnknown
		r.Headline, r.Verdict = blindVerdict(r, a, targets)
		blind = true
	default:
		r.Risk = RiskLow
		r.Headline = "no unusual timing"
		if r.AccountsChecked == "all" {
			r.Headline = "no unusual pattern"
		}
		r.Verdict = lowVerdict(r, a, targets, cleared)
	}

	// A repo nobody could judge by its stars still gets its forks and
	// issues in the verdict, so the reader can weigh it.
	if blind {
		r.Verdict += " " + activitySentence(r)
	}

	// The findings are the evidence behind the verdict, most important first.
	for _, spike := range unusual {
		line := fmt.Sprintf("Unusual spike on %s: %s stars in one day. A normal day in the four weeks before had %s.",
			date(spike.Peak), Number(spike.PeakStars), normalStars(spike.NormalDay))
		if d, ok := dayAt(cleared, spike.Peak); ok && d.level() == levelNormal {
			line += fmt.Sprintf(" The accounts behind it look like most stargazers (%d%% one-off).", percent(d.Accounts.Share()))
		}
		r.Findings = append(r.Findings, line)
	}
	if b := a.Burst; b != nil {
		r.Findings = append(r.Findings, fmt.Sprintf(
			"%d%% of all stars arrived in 90 days, from %s to %s (about %s a day). "+
				"Before that the repo got %s, and in the last 90 days %s.",
			percent(b.Share), date(b.Start), date(b.End), Number(int(math.Round(b.DailyDuring))),
			rate(b.DailyBefore), rate(b.DailyAfter)))
	}
	if line := accountsFinding(r.Days); line != "" {
		r.Findings = append(r.Findings, line)
	}
	var overall *Profiles
	if r.Details != nil {
		overall = r.Details.Profiles
	}
	dayProfiles := false
	for _, d := range r.Days {
		if line := profileFinding(d, overall); line != "" {
			r.Findings = append(r.Findings, line)
			dayProfiles = true
		}
	}
	// A small repo has no busy day with enough stargazers to sum up, but a
	// maintainer who asked for --deep still gets the counts for all of them.
	if !dayProfiles && overall != nil && overall.Accounts > 0 {
		r.Findings = append(r.Findings, overallProfileFinding(*overall))
	}
	if r.Risk == RiskMedium && len(high) == 0 && len(elevated) == 0 {
		r.Findings = append(r.Findings, "Timing alone cannot tell a news story from bought stars. "+
			"The account check can help when GH Archive has data for those days.")
	}
	r.Limits = append(r.Limits, baseLimits(r, targets)...)
}

// accountsChecked says how many of the days that needed an account check
// got one that counts: "all", "some" or "none".
func accountsChecked(days []Day, targets []int) string {
	counted := 0
	for _, i := range targets {
		if days[i].Counts() {
			counted++
		}
	}
	switch {
	case counted == 0:
		return "none"
	case counted < len(targets):
		return "some"
	}
	return "all"
}

func timingVerdict(spikes int, burst bool) string {
	switch {
	case spikes > 0 && burst:
		return fmt.Sprintf("%s and most stars in one burst, see below.", spikeCount(spikes))
	case spikes > 0:
		return spikeCount(spikes) + ", see below."
	default:
		return "Most stars arrived in one burst, with quiet periods before and after. See below."
	}
}

func timingHeadline(spikes int) string {
	switch {
	case spikes == 1:
		return "1 unusual spike"
	case spikes > 1:
		return Number(spikes) + " unusual spikes"
	}
	return "one big burst"
}

func spikeCount(n int) string {
	if n == 1 {
		return "One unusual spike"
	}
	return Number(n) + " unusual spikes"
}

// timingBlind reports whether the timing of the stars says nothing useful
// on its own: the repo is too young to have quiet periods before and after
// a burst, or most of its stars came with its launch, which timing never
// judges.
func timingBlind(a stars.Analysis) bool {
	if a.Days < stars.BurstMinHistory {
		return true
	}
	launch := 0
	for _, spike := range a.Spikes {
		if spike.Launch {
			launch += spike.Stars
		}
	}
	return 2*launch >= a.Total
}

// blindVerdict explains why a repo could not be judged: the timing cannot
// tell, and the accounts were not checked.
func blindVerdict(r *Result, a stars.Analysis, targets []int) (headline, verdict string) {
	headline = "not enough data"
	verdict = "Not enough data to judge. Most stars arrived with the launch, and timing alone cannot tell a launch from a star campaign. "
	if a.Days < stars.BurstMinHistory {
		headline = "too new to judge"
		verdict = fmt.Sprintf("Too new to judge. The repo is %s old, and timing alone cannot tell a launch from a star campaign. ",
			plural(a.Days, "day", "days"))
	}
	var missing []Day
	for _, i := range targets {
		missing = append(missing, r.Days[i])
	}
	busiest := "the busiest days"
	if len(targets) == 1 {
		busiest = "the busiest day"
	}
	switch {
	case len(targets) == 0:
		verdict += fmt.Sprintf("No day had %d or more stars, so the accounts were not checked.", ArchiveMinStars)
	case r.Sources.Archive == "off":
		verdict += "The accounts were not checked (--no-archive)."
	case r.Sources.Archive == "unavailable":
		verdict += "The account check in GH Archive did not work, see the limits."
	case onlyTooRecent(missing):
		verdict += fmt.Sprintf("The accounts behind %s can be checked after %s.", busiest, date(lastSettle(r.Days)))
	default:
		verdict += fmt.Sprintf("GH Archive has too few records of %s to check the accounts.", busiest)
	}
	return headline, verdict
}

// lowVerdict describes a repo where nothing unusual showed. When account
// checks are missing, it says so, because then only the timing was judged.
// Cleared holds the peaks of unusual spikes whose accounts were checked.
func lowVerdict(r *Result, a stars.Analysis, targets []int, cleared []Day) string {
	if a.Total < ArchiveMinStars && len(a.Spikes) == 0 {
		return fmt.Sprintf("No unusual pattern in the star timing, with %s so far. That is too few stars to check the accounts behind them.",
			plural(a.Total, "star", "stars"))
	}
	biggest := r.BiggestDay
	var missing []Day
	for _, i := range targets {
		if !r.Days[i].Counts() {
			missing = append(missing, r.Days[i])
		}
	}
	biggestChecked := biggest != nil && biggest.Counts()
	summary := timingSummary(biggest)
	if len(cleared) > 0 {
		summary = clearedSummary(cleared)
		biggestChecked = true
	}
	if len(missing) == 0 && biggestChecked {
		return summary + "."
	}

	var checks string
	switch {
	case len(targets) == 0:
		checks = fmt.Sprintf("No day had %d or more stars, so the accounts were not checked.", ArchiveMinStars)
	case r.Sources.Archive == "off":
		checks = "The accounts behind the busiest days were not checked."
	default:
		what := fmt.Sprintf("%d of the %d busiest days", len(missing), len(targets))
		switch {
		case len(missing) == len(targets) && len(targets) > 1:
			what = "the busiest days"
		case len(missing) == 1 && biggest != nil && missing[0].Day.Equal(biggest.Day) && biggest.Spike == "launch":
			what = "the launch day"
		case len(missing) == 1 && biggest != nil && missing[0].Day.Equal(biggest.Day):
			what = "the biggest day"
		case len(missing) == 1:
			what = date(missing[0].Day)
		}
		checks = "The accounts behind " + what + " could not be checked"
		if onlyTooRecent(missing) {
			checks += " yet"
		}
		checks += "."
	}
	if !biggestChecked {
		return "No unusual pattern in the star timing. " + checks
	}
	return summary + ". " + checks
}

// timingSummary says what the biggest day looked like. A big multiplier
// does not belong next to "no unusual pattern", so a spike gets its stars
// and date only; the list of busy days shows the rest.
func timingSummary(biggest *Day) string {
	switch {
	case biggest == nil:
		return "No unusual pattern"
	case biggest.Spike == "launch":
		return fmt.Sprintf("No unusual pattern. The biggest day was the launch on %s, with %s stars", date(biggest.Day), Number(biggest.Stars))
	case biggest.Spike == "":
		return fmt.Sprintf("No unusual pattern. The stars arrived steadily, and the biggest day had %s stars, %sx a normal day",
			Number(biggest.Stars), RatioText(biggest.Ratio))
	}
	return fmt.Sprintf("No unusual pattern. The biggest day was %s, with %s stars", date(biggest.Day), Number(biggest.Stars))
}

// clearedSummary says that the unusual spikes had normal accounts.
func clearedSummary(days []Day) string {
	var shares []string
	for _, d := range days {
		shares = append(shares, fmt.Sprintf("%d%%", percent(d.Accounts.Share())))
	}
	if len(days) == 1 {
		return fmt.Sprintf("No unusual pattern. The accounts behind the unusual spike on %s look like most stargazers (%s one-off)",
			date(days[0].Day), shares[0])
	}
	return fmt.Sprintf("No unusual pattern. The accounts behind the %d unusual spikes look like most stargazers (%s one-off)",
		len(days), joinAnd(shares))
}

// onlyTooRecent reports whether every day was missing only because its
// 30 days after are not over yet.
func onlyTooRecent(days []Day) bool {
	for _, d := range days {
		if d.Accounts == nil || !d.Accounts.Usable() || !d.Accounts.Provisional {
			return false
		}
	}
	return len(days) > 0
}

// accountSentence describes the share of one-off accounts on some days. A
// share from fewer than MinSample accounts comes with its sample size, so
// nobody reads more into it than it holds.
func accountSentence(days []Day) string {
	if len(days) == 1 {
		c := days[0].Accounts
		if c.Checked < MinSample {
			return fmt.Sprintf("On %s, %s of the %s accounts that starred (%d%%) had no other public stars recorded in GH Archive within %d days",
				date(days[0].Day), Number(c.OneOff), Number(c.Checked), percent(c.Share()), archive.WindowDays)
		}
		return fmt.Sprintf("On %s, %d%% of the accounts that starred had no other public stars recorded in GH Archive within %d days",
			date(days[0].Day), percent(c.Share()), archive.WindowDays)
	}
	low, highest := 1.0, 0.0
	var small []string
	for _, d := range days {
		share := d.Accounts.Share()
		low, highest = math.Min(low, share), math.Max(highest, share)
		if d.Accounts.Checked < MinSample {
			small = append(small, Number(d.Accounts.Checked))
		}
	}
	sentence := fmt.Sprintf("On %d of the busiest days, %d%% to %d%% of the accounts that starred had no other public stars recorded in GH Archive within %d days",
		len(days), percent(low), percent(highest), archive.WindowDays)
	if len(small) > 0 {
		sentence += fmt.Sprintf(" (some days had only %s accounts to check)", joinAnd(small))
	}
	return sentence
}

// accountsFinding sums up the GH Archive checks in one line.
func accountsFinding(days []Day) string {
	var parts []string
	worst := levelNone
	for _, d := range days {
		if d.Counts() {
			parts = append(parts, fmt.Sprintf("%d%% on %s", percent(d.Accounts.Share()), date(d.Day)))
			worst = max(worst, d.level())
		}
	}
	if len(parts) == 0 {
		return ""
	}
	// The numbers come from the calibration in docs/how-it-works.md.
	judgement := "For comparison, organic developer tools in the calibration set had up to 21%, " +
		"and viral lists and repos about a cause up to 38%."
	switch worst {
	case levelHigh:
		judgement = "No organic repo in the calibration set reached 40%. Most days of repos flagged by research were above 50%."
	case levelElevated:
		judgement = "That is more than organic developer tools in the calibration set (up to 21%). " +
			"Viral lists and repos about a cause reached 38% there, so this share alone proves little."
	}
	return fmt.Sprintf("Accounts with no other public stars recorded in GH Archive within %d days: %s. %s",
		archive.WindowDays, joinAnd(parts), judgement)
}

func profileFinding(d Day, overall *Profiles) string {
	p := d.Profiles
	if p == nil || p.Accounts < MinProfiles {
		return ""
	}
	line := fmt.Sprintf("Of the %s accounts that starred on %s, %d%% were created in the %d days before and %d%% have no followers and no public repositories.",
		Number(p.Accounts), date(d.Day), percent(float64(p.New)/float64(p.Accounts)), NewAccountDays, percent(float64(p.Empty)/float64(p.Accounts)))
	if overall != nil && overall.Accounts > 0 {
		line += fmt.Sprintf(" For all %s stargazers, the shares are %d%% and %d%%.", Number(overall.Accounts),
			percent(float64(overall.New)/float64(overall.Accounts)), percent(float64(overall.Empty)/float64(overall.Accounts)))
	}
	return line
}

// overallProfileFinding sums up the profiles of all stargazers. Counts
// under MinSample read better as numbers than as shares.
func overallProfileFinding(p Profiles) string {
	if p.Accounts < MinSample {
		created := fmt.Sprintf("%d were created in the %d days before they starred", p.New, NewAccountDays)
		if p.New == 1 {
			created = fmt.Sprintf("1 was created in the %d days before it starred", NewAccountDays)
		}
		empty := fmt.Sprintf("%d have no followers and no public repositories", p.Empty)
		if p.Empty == 1 {
			empty = "1 has no followers and no public repositories"
		}
		return fmt.Sprintf("Of all %s, %s, and %s.", plural(p.Accounts, "stargazer", "stargazers"), created, empty)
	}
	return fmt.Sprintf("Of all %s stargazers, %d%% were created in the %d days before they starred, and %d%% have no followers and no public repositories.",
		Number(p.Accounts), percent(float64(p.New)/float64(p.Accounts)), NewAccountDays, percent(float64(p.Empty)/float64(p.Accounts)))
}

func joinAnd(parts []string) string {
	if len(parts) <= 2 {
		return strings.Join(parts, " and ")
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

func baseLimits(r *Result, targets []int) []string {
	var limits []string
	if r.Stars > 0 {
		limits = append(limits,
			"Anyone can star any repo, so a pattern here says nothing about who arranged it.",
			"GitHub only counts stars that still exist. Stars from deleted accounts are missing from the history.")
	}
	if r.HistoryPartial {
		limits = append(limits, fmt.Sprintf("The star history only covers %s to %s: %s.", date(r.HistoryFrom), date(r.HistoryTo), r.Sources.HistoryProblem))
	}
	switch r.Sources.Archive {
	case "used", "partial":
		limits = append(limits, "Account checks use GH Archive, a public third-party record of GitHub events. "+
			"Since June 2025 it has missed a growing share of all stars, and since June 2026 almost all of them.")
	case "off":
		if len(targets) > 0 {
			limits = append(limits, "The accounts were not checked (--no-archive), so the verdict rests on the timing of the stars.")
		}
	}
	if r.Sources.ArchiveProblem != "" {
		limits = append(limits, "No account check from GH Archive: "+r.Sources.ArchiveProblem+".")
	}
	var thin, gaps, none, early, recent []string
	for _, d := range r.Days {
		switch c := d.Accounts; {
		case c == nil:
		case !c.Usable() && d.Day.Before(archiveStart):
			early = append(early, date(d.Day))
		case c.Gap():
			gaps = append(gaps, date(d.Day))
		case !c.Usable():
			thin = append(thin, fmt.Sprintf("%s (%s of %s stars)", date(d.Day), Number(c.Archived), Number(c.GitHubStars)))
			if c.Archived == 0 {
				none = append(none, date(d.Day))
			}
		case c.Provisional:
			recent = append(recent, date(d.Day))
		}
	}
	if len(thin) > 0 {
		limits = append(limits, "GH Archive has too few records to check the accounts on "+joinAnd(thin)+".")
	}
	if len(none) > 0 {
		// GH Archive files stars under the name a repo had at the time, so
		// no record at all often means the repo had another name.
		limits = append(limits, fmt.Sprintf("GH Archive has no stars at all for this repo on %s. If the repo had another name then, "+
			"check it under that name, because GH Archive files stars under the name a repo had at the time.", joinAnd(none)))
	}
	if len(gaps) > 0 {
		that, it := "that day", "it"
		if len(gaps) > 1 {
			that, it = "those days", "them"
		}
		limits = append(limits, fmt.Sprintf("GH Archive missed many stars across GitHub in the %d days before and after %s. "+
			"The accounts from %s could look one-off only because their other stars are missing, so StarTrust did not judge %s.",
			archive.WindowDays, joinAnd(gaps), that, it))
	}
	if len(early) > 0 {
		limits = append(limits, "GH Archive starts in February 2011, so it has no record of "+joinAnd(early)+".")
	}
	if len(recent) > 0 {
		limits = append(limits, fmt.Sprintf("The account check for %s is provisional and does not count yet, "+
			"because an account can still star another repo in the %d days after. Check again after %s.",
			joinAnd(recent), archive.WindowDays, date(lastSettle(r.Days))))
	}
	if r.Sources.IssuesProblem != "" {
		limits = append(limits, "GitHub's search did not answer, so the recent issues and pull requests were not counted: "+
			r.Sources.IssuesProblem+".")
	}
	if r.Sources.TokenProblem != "" {
		limits = append(limits, r.Sources.TokenProblem+".")
	}
	if r.Sources.StargazerReason != "" {
		limits = append(limits, r.Sources.StargazerReason+".")
	}
	return limits
}

// lastSettle returns the first day on which every provisional check counts.
func lastSettle(days []Day) time.Time {
	var last time.Time
	for _, d := range days {
		if d.Accounts != nil && d.Accounts.Provisional {
			if settle := archive.SettledOn(d.Day); settle.After(last) {
				last = settle
			}
		}
	}
	return last
}

// archiveStart is the first day GH Archive has events for.
var archiveStart = time.Date(2011, 2, 12, 0, 0, 0, 0, time.UTC)

func dayAt(days []Day, day time.Time) (Day, bool) {
	for _, d := range days {
		if d.Day.Equal(day) {
			return d, true
		}
	}
	return Day{}, false
}

func date(t time.Time) string { return t.Format("2006-01-02") }

func percent(share float64) int { return int(math.Round(share * 100)) }

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// RatioText formats a "times normal" ratio. It rounds to one decimal first,
// like the JSON, so a stored ratio and a fresh one always print the same.
func RatioText(r float64) string {
	r = round1(r)
	if r < 10 {
		return fmt.Sprintf("%.0f", math.Max(1, math.Round(r)))
	}
	return Number(int(math.Round(r)))
}

// normalStars says how many stars a normal day had, such as "under 1 star"
// or "12 stars".
func normalStars(normal float64) string {
	if normal < 1 {
		return "under 1 star"
	}
	return plural(int(math.Round(normal)), "star", "stars")
}

func rate(perDay float64) string {
	switch {
	case perDay < 0.1:
		return "almost no stars"
	case perDay < 1:
		return fmt.Sprintf("about one star every %d days", int(math.Round(1/perDay)))
	}
	return "about " + Number(int(math.Round(perDay))) + " stars a day"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return Number(n) + " " + many
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Number formats an integer with thousands separators.
func Number(n int) string {
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	digits := fmt.Sprint(n)
	var out strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(d)
	}
	return sign + out.String()
}
