package archive

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// TestRealClickHouse runs the cohort query on a real ClickHouse server with
// a small github_events table. The fake servers in the other tests only
// check what the client sends; this test lets the database run the SQL.
//
// Run it with scripts/clickhouse-test.sh, which starts a server in Docker.
// It needs STARTRUST_CLICKHOUSE_URL pointing at a server where the user may
// create tables, such as http://127.0.0.1:8123/?user=default.
func TestRealClickHouse(t *testing.T) {
	endpoint := os.Getenv("STARTRUST_CLICKHOUSE_URL")
	if endpoint == "" {
		t.Skip("set STARTRUST_CLICKHOUSE_URL to run the SQL on a real ClickHouse server")
	}
	exec := func(query, body string) {
		t.Helper()
		u, err := url.Parse(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		params := u.Query()
		params.Set("query", query)
		u.RawQuery = params.Encode()
		res, err := http.Post(u.String(), "text/plain", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		answer, _ := io.ReadAll(res.Body)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s: HTTP %d: %s", query, res.StatusCode, answer)
		}
	}
	exec("DROP TABLE IF EXISTS github_events", "")
	// The columns the query reads, sorted like the public table.
	exec(`CREATE TABLE github_events (
		event_type Enum8('WatchEvent' = 1, 'PushEvent' = 2),
		actor_login LowCardinality(String),
		repo_name LowCardinality(String),
		created_at DateTime
	) ENGINE = MergeTree ORDER BY (event_type, repo_name, created_at)`, "")

	launch := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	gapDay := time.Date(2025, 8, 10, 0, 0, 0, 0, time.UTC)
	var rows []string
	star := func(actor, repo string, at time.Time) {
		rows = append(rows, fmt.Sprintf("WatchEvent\t%s\t%s\t%s", actor, repo, at.Format("2006-01-02 15:04:05")))
	}
	// 100 accounts star me/tool at 13:00 Pacific time on launch day. The
	// first 60 also star another repo five days later, which keeps them
	// from being one-off. Ten more star another repo 40 days later, outside
	// the window, so they still count as one-off.
	for i := 0; i < 100; i++ {
		actor := fmt.Sprintf("user%03d", i)
		star(actor, "me/tool", launch.Add(20*time.Hour))
		switch {
		case i < 60:
			star(actor, "x/other", launch.AddDate(0, 0, 5))
		case i < 70:
			star(actor, "x/other", launch.AddDate(0, 0, 40))
		}
	}
	// Stars just before and after the Pacific day do not count.
	star("early", "me/tool", launch.Add(6*time.Hour+30*time.Minute))
	star("late", "me/tool", launch.Add(32*time.Hour))
	// Twenty more accounts starred the repo under its old name.
	for i := 0; i < 20; i++ {
		star(fmt.Sprintf("old%02d", i), "me/old-tool", launch.Add(18*time.Hour))
	}
	// Background stars across GitHub: 100 a day around launch day. Around
	// the gap day GH Archive has 100 a day in its week and only 5 a day in
	// the rest of the window.
	for d := -40; d <= 40; d++ {
		for i := 0; i < 100; i++ {
			star(fmt.Sprintf("bg%d-%d", d, i), "bg/repo", launch.AddDate(0, 0, d).Add(time.Duration(i)*time.Minute))
		}
		n := 5
		if d >= -3 && d <= 3 {
			n = 100
		}
		for i := 0; i < n; i++ {
			star(fmt.Sprintf("gap%d-%d", d, i), "bg/repo", gapDay.AddDate(0, 0, d).Add(time.Duration(i)*time.Minute))
		}
	}
	for i := 0; i < 50; i++ {
		star(fmt.Sprintf("gapuser%02d", i), "me/gap", gapDay.Add(20*time.Hour))
	}
	exec("INSERT INTO github_events FORMAT TSV", strings.Join(rows, "\n")+"\n")

	// The expected window numbers, counted the way the query counts them.
	recorded := func(day time.Time) float64 {
		window, week := 0, 0
		for _, row := range rows {
			at, _ := time.Parse("2006-01-02 15:04:05", strings.Split(row, "\t")[3])
			if !at.Before(day.AddDate(0, 0, -WindowDays)) && at.Before(day.AddDate(0, 0, WindowDays+1)) {
				window++
			}
			if !at.Before(day.AddDate(0, 0, -3)) && at.Before(day.AddDate(0, 0, 4)) {
				week++
			}
		}
		return windowRecorded(window, week)
	}

	client := &Client{Endpoint: endpoint}
	today := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()

	cohort, err := client.Cohort(ctx, []string{"me/tool"}, launch, 100, today)
	if err != nil {
		t.Fatal(err)
	}
	want := Cohort{Day: launch, GitHubStars: 100, Archived: 100, Checked: 100, OneOff: 40, WindowRecorded: recorded(launch), Judged: true}
	if cohort != want {
		t.Errorf("cohort = %+v\nwant     %+v", cohort, want)
	}

	cohort, err = client.Cohort(ctx, []string{"me/tool", "me/old-tool"}, launch, 120, today)
	if err != nil {
		t.Fatal(err)
	}
	if cohort.Archived != 120 || cohort.Checked != 120 || cohort.OneOff != 60 {
		t.Errorf("under both names: %+v", cohort)
	}

	cohort, err = client.Cohort(ctx, []string{"me/gap"}, gapDay, 50, today)
	if err != nil {
		t.Fatal(err)
	}
	if cohort.WindowRecorded != recorded(gapDay) || cohort.WindowRecorded > 0.2 || !cohort.Gap() || cohort.Judged || cohort.Checked != 0 {
		t.Errorf("a day next to a gap must not be judged: %+v", cohort)
	}
}
