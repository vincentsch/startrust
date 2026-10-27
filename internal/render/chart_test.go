package render

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/startrust/internal/check"
	"github.com/vincentsch/startrust/internal/github"
	"github.com/vincentsch/startrust/internal/stars"
)

func series(start string, daily []int) stars.Series {
	t, _ := time.Parse("2006-01-02", start)
	return stars.Series{Start: t, Daily: daily}
}

func TestUnitFollowsAge(t *testing.T) {
	cases := []struct {
		days int
		want string
	}{
		{30, "day"},
		{64, "day"},
		{65, "week"},
		{400, "week"},
		{1000, "month"},
		{4760, "quarter"},
	}
	for _, c := range cases {
		if got := ChooseUnit(series("2013-10-23", make([]int, c.days))).Name; got != c.want {
			t.Errorf("%d days: %s, want %s", c.days, got, c.want)
		}
	}
}

func TestBucketsStartOnCalendarBoundaries(t *testing.T) {
	s := series("2025-01-15", make([]int, 100))
	s.Daily[0], s.Daily[20], s.Daily[99] = 1, 2, 3
	buckets := MakeBuckets(s, Unit{Name: "month", Months: 1})
	if len(buckets) != 4 || buckets[0].Start.Format("2006-01-02") != "2025-01-01" || buckets[1].Stars != 2 {
		t.Fatalf("buckets = %+v", buckets)
	}
	weeks := MakeBuckets(s, Unit{Name: "week", Days: 7})
	if weeks[0].Start.Weekday() != time.Monday {
		t.Errorf("weeks start on %s", weeks[0].Start.Weekday())
	}
}

func TestChartShape(t *testing.T) {
	daily := make([]int, 30)
	daily[10] = 80
	daily[11] = 40
	daily[25] = 1
	chart := Chart(series("2026-10-01", daily), []Marker{{Day: daily0(10), Label: "1"}})
	lines := strings.Split(strings.TrimRight(chart, "\n"), "\n")
	if lines[0] != "Stars per day, 2026-10-01 to 2026-10-30" {
		t.Errorf("title %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "80 ┤") {
		t.Errorf("top line should carry the peak: %q", lines[1])
	}
	bottom := lines[chartRows]
	if !strings.Contains(bottom, "▁") {
		t.Errorf("a single star must still show: %q", bottom)
	}
	if !strings.Contains(lines[len(lines)-1], "1") {
		t.Errorf("marker missing: %q", lines[len(lines)-1])
	}
	for _, line := range lines {
		if n := len([]rune(line)); n > 80 {
			t.Errorf("line is %d columns wide: %q", n, line)
		}
	}
}

func daily0(i int) time.Time {
	t, _ := time.Parse("2006-01-02", "2026-10-01")
	return t.AddDate(0, 0, i)
}

func TestNeighbouringMarkersDoNotMerge(t *testing.T) {
	daily := make([]int, 3*365)
	s := series("2023-01-01", daily)
	chart := Chart(s, []Marker{{Day: s.Day(400), Label: "1"}, {Day: s.Day(430), Label: "2"}})
	lines := strings.Split(strings.TrimRight(chart, "\n"), "\n")
	markers := strings.Join(lines[len(lines)-2:], "\n")
	if strings.Contains(markers, "12") || !strings.Contains(markers, "1") || !strings.Contains(markers, "2") {
		t.Errorf("markers ran together:\n%s", chart)
	}
}

func TestSparkline(t *testing.T) {
	got := Sparkline([]Bucket{{Stars: 0}, {Stars: 1}, {Stars: 50}, {Stars: 100}})
	if got != " ▁▄█" {
		t.Errorf("sparkline %q", got)
	}
}

func TestWrap(t *testing.T) {
	got := wrap("- one two three four five", 12, "  ")
	if got != "- one two\n  three four\n  five" {
		t.Errorf("wrap = %q", got)
	}
}

func TestEveryMarkerIsShown(t *testing.T) {
	// Four busy days in one month and one in the next: all five numbers
	// must show, even where they share a bar.
	s := series("2023-01-01", make([]int, 3*365))
	var markers []Marker
	for i, d := range []int{400, 405, 410, 420, 432} {
		markers = append(markers, Marker{Day: s.Day(d), Label: fmt.Sprint(i + 1)})
	}
	chart := Chart(s, markers)
	lines := strings.Split(strings.TrimRight(chart, "\n"), "\n")
	below := strings.Join(lines[chartRows+3:], "\n")
	for i := 1; i <= 5; i++ {
		if !strings.Contains(below, fmt.Sprint(i)) {
			t.Errorf("marker %d is missing:\n%s", i, chart)
		}
	}
}

func TestYearLabelsWinOverTheStartLabel(t *testing.T) {
	// A history from November 2024: the 2025 and 2026 labels must both
	// show, even though the start of the chart is right before 2025.
	s := series("2024-11-15", make([]int, 711))
	chart := Chart(s, nil)
	axis := strings.Split(strings.TrimRight(chart, "\n"), "\n")[chartRows+2]
	if !strings.Contains(axis, "2025") || !strings.Contains(axis, "2026") {
		t.Errorf("year labels: %q", axis)
	}
}

func TestDayChartsHaveWeeklyLabels(t *testing.T) {
	s := series("2026-09-30", make([]int, 27))
	axis := strings.Split(strings.TrimRight(Chart(s, nil), "\n"), "\n")[chartRows+2]
	for _, want := range []string{"Oct 1", "Oct 5", "Oct 12", "Oct 19"} {
		if !strings.Contains(axis, want) {
			t.Errorf("day chart axis is missing %q: %q", want, axis)
		}
	}
}

// TestRealHistoriesFit draws every captured history with its busy days.
// Each chart must fit in 80 columns, show every busy day's number and
// label more than one year when it spans several.
func TestRealHistoriesFit(t *testing.T) {
	files, err := filepath.Glob("../stars/testdata/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	for _, file := range files {
		var f struct {
			Repo struct {
				CreatedAt time.Time `json:"created_at"`
			} `json:"repo"`
			Weeks []github.Week `json:"weeks"`
		}
		raw, _ := os.ReadFile(file)
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatal(err)
		}
		s := stars.FromWeeks(f.Weeks, f.Repo.CreatedAt, now)
		days := check.BiggestDays(s, stars.Analyze(s), check.BigDays)
		var markers []Marker
		for i, d := range days {
			markers = append(markers, Marker{Day: d.Day, Label: fmt.Sprint(i + 1)})
		}
		chart := Chart(s, markers)
		lines := strings.Split(strings.TrimRight(chart, "\n"), "\n")
		for _, line := range lines {
			if n := len([]rune(line)); n > 80 {
				t.Errorf("%s: line is %d columns wide: %q", file, n, line)
			}
		}
		below := " " + strings.ReplaceAll(strings.Join(lines[chartRows+3:], " "), ",", " ") + " "
		for _, m := range markers {
			if !strings.Contains(below, " "+m.Label+" ") {
				t.Errorf("%s: marker %s is missing:\n%s", file, m.Label, chart)
			}
		}
		if years := s.End().Year() - s.Start.Year(); years >= 2 {
			labels := strings.Fields(lines[chartRows+2])
			if len(labels) < 2 {
				t.Errorf("%s: %d years with only these labels: %v", file, years, labels)
			}
		}
	}
}

func TestWrapCountsTheIndentOfTheFirstLine(t *testing.T) {
	got := wrap("    one two three", 12, "      ")
	if got != "    one two\n      three" {
		t.Errorf("wrap = %q", got)
	}
}
