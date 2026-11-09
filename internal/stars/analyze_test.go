package stars

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/startrust/internal/github"
)

func TestFromWeeksTrimsFutureAndPreCreationDays(t *testing.T) {
	sunday := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC).Unix()
	weeks := []github.Week{
		{Week: sunday + 7*86400, Total: 3, Days: [7]int{1, 2, 0, 0, 0, 0, 0}}, // newest first, as GitHub sends
		{Week: sunday, Total: 9, Days: [7]int{0, 0, 4, 5, 0, 0, 0}},
	}
	created := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC) // a Tuesday
	today := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)    // the next Monday
	s := FromWeeks(weeks, created, today)
	if got := s.Start.Format("2006-01-02"); got != "2026-10-20" {
		t.Errorf("start %s, want the creation day", got)
	}
	if got := s.End().Format("2006-01-02"); got != "2026-10-26" {
		t.Errorf("end %s, want today", got)
	}
	if s.Total() != 12 {
		t.Errorf("total %d, want 12", s.Total())
	}
}

func TestFromWeeksKeepsStarsFromBeforeCreation(t *testing.T) {
	// A transferred or recreated repo can hold stars older than its
	// creation date. They must still count.
	sunday := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC).Unix()
	weeks := []github.Week{{Week: sunday, Total: 5, Days: [7]int{5, 0, 0, 0, 0, 0, 0}}}
	s := FromWeeks(weeks, time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC))
	if s.Total() != 5 || s.Start.Format("2006-01-02") != "2026-10-18" {
		t.Errorf("got start %s total %d", s.Start.Format("2006-01-02"), s.Total())
	}
}

func TestSpikeOnQuietRepo(t *testing.T) {
	// 200 quiet days with about one star, then 500 stars in a day: an
	// unusual spike (not a launch: the repo already had over 100 stars).
	daily := make([]int, 300)
	for i := range daily[:200] {
		daily[i] = i % 2
	}
	daily[200] = 500
	daily[201] = 120
	s := Series{Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Daily: daily}
	a := Analyze(s)
	if len(a.Spikes) != 1 {
		t.Fatalf("spikes = %+v", a.Spikes)
	}
	spike := a.Spikes[0]
	if !spike.Unusual || spike.Launch || spike.PeakStars != 500 || spike.Stars != 620 {
		t.Errorf("spike = %+v", spike)
	}
}

func TestLaunchIsNotUnusual(t *testing.T) {
	daily := make([]int, 100)
	daily[10] = 900
	daily[11] = 500
	for i := 12; i < 100; i++ {
		daily[i] = 20
	}
	a := Analyze(Series{Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Daily: daily})
	if len(a.Spikes) == 0 || !a.Spikes[0].Launch || a.Spikes[0].Unusual {
		t.Errorf("spikes = %+v", a.Spikes)
	}
}

func TestEmptySeries(t *testing.T) {
	a := Analyze(Series{})
	if a.Total != 0 || len(a.Spikes) != 0 || a.Burst != nil {
		t.Errorf("analysis of nothing = %+v", a)
	}
}

func TestMergedSpikeUsesTheNormalDayOfItsPeak(t *testing.T) {
	// 200 days with one star, 30 days with 30 stars, then 600 stars. The
	// 30-star days start a spike, and the 600-star day joins it as its new
	// peak. The peak must be compared with the 30-star days before it
	// (20 times), not with the quiet days before the spike began.
	var daily []int
	for range 200 {
		daily = append(daily, 1)
	}
	for range 30 {
		daily = append(daily, 30)
	}
	daily = append(daily, 600)
	for range 50 {
		daily = append(daily, 5)
	}
	a := Analyze(Series{Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Daily: daily})
	if len(a.Spikes) != 1 {
		t.Fatalf("spikes = %+v", a.Spikes)
	}
	spike := a.Spikes[0]
	if spike.PeakStars != 600 || spike.NormalDay != 30 || spike.Ratio == nil || *spike.Ratio != 20 || spike.Unusual {
		t.Errorf("peak %d, normal day %v, ratio %v, unusual %v", spike.PeakStars, spike.NormalDay, spike.Ratio, spike.Unusual)
	}
}

func TestSpikeAfterDaysWithoutStarsHasNoRatio(t *testing.T) {
	// 200 stars in the first ten days, then nothing for months, then 400
	// stars in a day. The normal day before it is 0, so a ratio would only
	// repeat the star count. The spike is still unusual: for the rules, a
	// normal day under 1 star counts as 1.
	daily := make([]int, 200)
	for i := range daily[:10] {
		daily[i] = 20
	}
	daily[150] = 400
	a := Analyze(Series{Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Daily: daily})
	if len(a.Spikes) != 1 {
		t.Fatalf("spikes = %+v", a.Spikes)
	}
	spike := a.Spikes[0]
	if spike.NormalDay != 0 || spike.Ratio != nil || !spike.Unusual || spike.Launch {
		t.Errorf("normal day %v, ratio %v, unusual %v, launch %v", spike.NormalDay, spike.Ratio, spike.Unusual, spike.Launch)
	}
	data, _ := json.Marshal(spike)
	if !strings.Contains(string(data), `"times_normal":null`) {
		t.Errorf("JSON: %s", data)
	}
}

func TestBurstNeedsEnoughStars(t *testing.T) {
	quietRepo := func(burst []int) Series {
		daily := make([]int, 500)
		copy(daily[200:], burst)
		return Series{Start: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Daily: daily}
	}
	// One star on one day of a quiet repo is most of its stars, but it is
	// not a burst worth a warning.
	if b := Analyze(quietRepo([]int{1})).Burst; b != nil {
		t.Errorf("one star made a burst: %+v", b)
	}
	if b := Analyze(quietRepo([]int{500, 400, 99})).Burst; b != nil {
		t.Errorf("999 stars made a burst: %+v", b)
	}
	if b := Analyze(quietRepo([]int{500, 400, 100})).Burst; b == nil || b.Stars != BurstMinStars {
		t.Errorf("1,000 stars between quiet periods should be a burst: %+v", b)
	}
}
