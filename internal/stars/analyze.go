package stars

import (
	"sort"
	"time"
)

// The numbers behind spike and burst detection. docs/how-it-works.md
// explains each one and shows how the calibration repos score against them.
const (
	// A day is a spike when it has at least SpikeMinStars stars and at
	// least SpikeRatio times a normal day.
	SpikeMinStars = 30
	SpikeRatio    = 10.0

	// A normal day is the median of the BaselineDays days before.
	BaselineDays = 28

	// Spike days at most MergeGapDays apart count as one spike.
	MergeGapDays = 3

	// A spike is a launch when the repo had fewer than LaunchStars stars
	// before it. Launches are expected to jump from nothing.
	LaunchStars = 100

	// A spike that is not a launch is unusual when its biggest day has at
	// least UnusualRatio times a normal day. Organic repos also reach that
	// when they go viral long after they were created (up to 2,919 times
	// in the calibration set), so an unusual spike alone never makes the
	// risk high.
	UnusualRatio = 300.0

	// A burst means that at least BurstShare of all stars and at least
	// BurstMinStars stars arrived in BurstDays days, and both before and
	// after that window the repo got under BurstQuiet of the burst's daily
	// rate. The repo must have existed for BurstMinBefore days before the
	// burst, and the burst must have ended BurstDays days ago, so both quiet
	// periods can be measured. Small repos often get most of their stars
	// from one post somewhere, so a burst of a few hundred stars says
	// nothing.
	BurstDays      = 90
	BurstShare     = 0.5
	BurstMinStars  = 1000
	BurstQuiet     = 0.05
	BurstMinBefore = 60
	burstBeforeMax = 180

	// BurstMinHistory is the youngest repo, in days, in which a burst can
	// be found at all.
	BurstMinHistory = BurstMinBefore + 2*BurstDays
)

// Spike is a run of unusually busy days.
type Spike struct {
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	Peak        time.Time `json:"peak_day"`
	PeakStars   int       `json:"peak_stars"`
	Stars       int       `json:"stars"`
	NormalDay   float64   `json:"normal_day"`
	Ratio       float64   `json:"times_normal"`
	StarsBefore int       `json:"stars_before"`
	Launch      bool      `json:"launch"`
	Unusual     bool      `json:"unusual"`
}

// Burst describes the busiest BurstDays window when most stars arrived in it
// and the repo was quiet before and after.
type Burst struct {
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	Stars       int       `json:"stars"`
	Share       float64   `json:"share"`
	DailyDuring float64   `json:"daily_during"`
	DailyBefore float64   `json:"daily_before"`
	DailyAfter  float64   `json:"daily_after"`
}

// Analysis is what the timing of stars shows.
type Analysis struct {
	Total           int       `json:"total"`
	Days            int       `json:"days"`
	BiggestDay      time.Time `json:"biggest_day"`
	BiggestDayStars int       `json:"biggest_day_stars"`
	// NormalDay is a normal day in the four weeks before the biggest day.
	NormalDay float64 `json:"normal_day"`
	// RecentDaily is the average stars per day over the last 30 days.
	RecentDaily float64 `json:"recent_daily"`
	Spikes      []Spike `json:"spikes"`
	Burst       *Burst  `json:"burst"`
}

// Unusual returns the unusual spikes.
func (a Analysis) Unusual() []Spike {
	var out []Spike
	for _, spike := range a.Spikes {
		if spike.Unusual {
			out = append(out, spike)
		}
	}
	return out
}

// Analyze finds spikes and bursts in a series.
func Analyze(s Series) Analysis {
	a := Analysis{Total: s.Total(), Days: len(s.Daily), Spikes: []Spike{}}
	if a.Total == 0 {
		return a
	}
	biggest := 0
	for i, n := range s.Daily {
		if n > s.Daily[biggest] {
			biggest = i
		}
	}
	a.BiggestDay = s.Day(biggest)
	a.BiggestDayStars = s.Daily[biggest]
	a.NormalDay = normalDay(s.Daily, biggest)
	recent := s.Daily[max(0, len(s.Daily)-30):]
	a.RecentDaily = float64(sum(recent)) / float64(len(recent))
	a.Spikes = findSpikes(s)
	a.Burst = findBurst(s)
	return a
}

// normalDay is the median of the BaselineDays days before day i.
func normalDay(daily []int, i int) float64 {
	return Median(daily[max(0, i-BaselineDays):i])
}

// Ratio compares a day to a normal day. A normal day of under one star
// counts as one, so a quiet repo's first 50-star day is 50 times normal.
func Ratio(stars int, normal float64) float64 {
	return float64(stars) / max(normal, 1)
}

func findSpikes(s Series) []Spike {
	var spikes []Spike
	starsBefore := 0
	for i, n := range s.Daily {
		normal := normalDay(s.Daily, i)
		day := s.Day(i)
		last := len(spikes) - 1
		// A spike also goes on while each next day has at least SpikeMinStars
		// stars and half the stars of its biggest day. Without this, a launch
		// week ends after day one, because day one already lifts the normal
		// day.
		continues := last >= 0 && day.Sub(spikes[last].End) == 24*time.Hour && n >= SpikeMinStars && 2*n >= spikes[last].PeakStars
		if continues || n >= SpikeMinStars && Ratio(n, normal) >= SpikeRatio {
			if last >= 0 && day.Sub(spikes[last].End) <= MergeGapDays*24*time.Hour {
				spike := &spikes[last]
				spike.Stars += sum(s.Daily[s.Index(spike.End)+1 : i+1])
				spike.End = day
				// The ratio compares the peak with the days before the
				// peak, so a new peak brings its own normal day.
				if n > spike.PeakStars {
					spike.Peak, spike.PeakStars, spike.NormalDay = day, n, normal
				}
			} else {
				spikes = append(spikes, Spike{
					Start: day, End: day, Peak: day,
					PeakStars: n, Stars: n,
					NormalDay: normal, StarsBefore: starsBefore,
				})
			}
		}
		starsBefore += n
	}
	for i := range spikes {
		spike := &spikes[i]
		spike.Ratio = Ratio(spike.PeakStars, spike.NormalDay)
		spike.Launch = spike.StarsBefore < LaunchStars
		spike.Unusual = !spike.Launch && spike.Ratio >= UnusualRatio
	}
	// Biggest first: that is the order people read them in.
	sort.SliceStable(spikes, func(i, j int) bool { return spikes[i].PeakStars > spikes[j].PeakStars })
	return spikes
}

func findBurst(s Series) *Burst {
	total := s.Total()
	if len(s.Daily) < BurstMinHistory {
		return nil
	}
	best, bestStart := -1, 0
	window := sum(s.Daily[:BurstDays])
	for start := 0; start+BurstDays <= len(s.Daily); start++ {
		if start > 0 {
			window += s.Daily[start+BurstDays-1] - s.Daily[start-1]
		}
		if window > best {
			best, bestStart = window, start
		}
	}
	end := bestStart + BurstDays // first day after the window
	if bestStart < BurstMinBefore || len(s.Daily)-end < BurstDays {
		return nil
	}
	before := s.Daily[max(0, bestStart-burstBeforeMax):bestStart]
	after := s.Daily[len(s.Daily)-BurstDays:]
	burst := &Burst{
		Start:       s.Day(bestStart),
		End:         s.Day(end - 1),
		Stars:       best,
		Share:       float64(best) / float64(total),
		DailyDuring: float64(best) / BurstDays,
		DailyBefore: float64(sum(before)) / float64(len(before)),
		DailyAfter:  float64(sum(after)) / float64(len(after)),
	}
	quiet := BurstQuiet * burst.DailyDuring
	if burst.Stars < BurstMinStars || burst.Share < BurstShare || burst.DailyBefore >= quiet || burst.DailyAfter >= quiet {
		return nil
	}
	return burst
}
