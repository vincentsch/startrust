package archive

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var (
	day   = time.Date(2025, 2, 10, 0, 0, 0, 0, time.UTC)
	today = time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
)

func fakeClickHouse(t *testing.T, status int, body string, check func(query string)) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Query().Get("readonly") != "1" {
			t.Errorf("query must run read-only")
		}
		query, _ := io.ReadAll(req.Body)
		if check != nil {
			check(string(query))
		}
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return &Client{Endpoint: server.URL + "/?user=play"}
}

func TestCohortReadsTheAnswer(t *testing.T) {
	client := fakeClickHouse(t, 200, `{"meta":[],"data":[["4800","1000","330","61000","7000"]],"rows":1}`, func(query string) {
		for _, want := range []string{"'owner/new-name'", "'owner/old-name'", "toDate('2025-02-10')", "America/Los_Angeles", "LIMIT 1000"} {
			if !strings.Contains(query, want) {
				t.Errorf("query is missing %s:\n%s", want, query)
			}
		}
	})
	cohort, err := client.Cohort(context.Background(), []string{"owner/new-name", "owner/old-name"}, day, 4500, today)
	if err != nil {
		t.Fatal(err)
	}
	if !cohort.Judged() || cohort.Archived != 4800 || cohort.Checked != 1000 || cohort.OneOff != 330 || cohort.WindowRecorded != 1 {
		t.Errorf("cohort = %+v", cohort)
	}
	if share := cohort.Share(); share != 0.33 {
		t.Errorf("share = %v", share)
	}
	if client.Queries() != 1 {
		t.Errorf("queries = %d", client.Queries())
	}
}

func TestCohortWithTooFewArchivedStarsIsNotUsable(t *testing.T) {
	client := fakeClickHouse(t, 200, `{"data":[[5,5,2,61000,7000]]}`, nil)
	cohort, err := client.Cohort(context.Background(), []string{"vincentsch/explainroo"}, day, 261, today)
	if err != nil {
		t.Fatal(err)
	}
	if cohort.Usable() || cohort.Judged() || !strings.Contains(cohort.Problem, "only 5 of the 261") {
		t.Errorf("cohort = %+v", cohort)
	}
	// Two of five accounts is not a share anybody should read.
	if cohort.Checked != 0 || cohort.OneOff != 0 {
		t.Errorf("a day that cannot be judged must not carry one-off counts: %+v", cohort)
	}
}

func TestCohortWithLowCoverageIsNotUsable(t *testing.T) {
	client := fakeClickHouse(t, 200, `{"data":[[100,100,3,61000,7000]]}`, nil)
	cohort, _ := client.Cohort(context.Background(), []string{"a/b"}, day, 1000, today)
	if cohort.Usable() {
		t.Errorf("100 of 1,000 stars should not be enough: %+v", cohort)
	}
}

func TestQuotaErrorIsExplained(t *testing.T) {
	client := fakeClickHouse(t, 500, "Code: 201. DB::Exception: Quota for user `play` for 3600s has been exceeded: read_rows = 300000971379/300000000000. (QUOTA_EXCEEDED)", nil)
	_, err := client.Cohort(context.Background(), []string{"a/b"}, day, 100, today)
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "hourly limit") {
		t.Fatalf("error = %v", err)
	}
}

func TestUnsafeNamesNeverReachTheQuery(t *testing.T) {
	client := fakeClickHouse(t, 200, `{"data":[[1,1,1,1,1]]}`, func(string) { t.Error("query sent") })
	_, err := client.Cohort(context.Background(), []string{"a/b' OR 1=1 --"}, day, 10, today)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestUnreachableEndpoint(t *testing.T) {
	client := &Client{Endpoint: "http://127.0.0.1:1/?user=play"}
	_, err := client.Cohort(context.Background(), []string{"a/b"}, day, 10, today)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestDayNextToAnArchiveGapIsNotJudged(t *testing.T) {
	// GH Archive has 90% of the day's stars, but recorded only 30% as many
	// stars a day across GitHub in the weeks around it. The accounts' other
	// stars would be missing, so they would look one-off.
	client := fakeClickHouse(t, 200, `{"data":[["900","900","600","18300","7000"]]}`, nil)
	cohort, err := client.Cohort(context.Background(), []string{"a/b"}, day, 1000, today)
	if err != nil {
		t.Fatal(err)
	}
	if cohort.WindowRecorded != 0.3 || cohort.Judged() || !cohort.Gap() || cohort.OneOff != 0 {
		t.Errorf("cohort = %+v", cohort)
	}
	if !strings.Contains(cohort.Problem, "only 30% as many stars across GitHub in the 30 days before and after") {
		t.Errorf("problem = %q", cohort.Problem)
	}

	// A smaller dip is fine when the day itself is well recorded: 90% of
	// the stars times 60% is still over the 40% that a day needs.
	client = fakeClickHouse(t, 200, `{"data":[["900","900","100","36600","7000"]]}`, nil)
	if cohort, _ := client.Cohort(context.Background(), []string{"a/b"}, day, 1000, today); !cohort.Judged() {
		t.Errorf("cohort = %+v", cohort)
	}
}

func TestRecentDayIsProvisional(t *testing.T) {
	// Yesterday's window is mostly in the future, so it looks thin. That
	// must make the day provisional, not a gap.
	client := fakeClickHouse(t, 200, `{"data":[["100","100","60","32000","7000"]]}`, nil)
	yesterday := today.AddDate(0, 0, -1)
	cohort, err := client.Cohort(context.Background(), []string{"a/b"}, yesterday, 100, today)
	if err != nil {
		t.Fatal(err)
	}
	if !cohort.Provisional || cohort.Problem != "" || cohort.Judged() || cohort.OneOff != 60 {
		t.Errorf("cohort = %+v", cohort)
	}
}

func TestCtrlCIsNotReportedAsATimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		io.ReadAll(req.Body)
		close(started)
		<-req.Context().Done()
	}))
	defer server.Close()
	client := &Client{Endpoint: server.URL}
	go func() {
		<-started
		cancel()
	}()
	_, err := client.Cohort(ctx, []string{"a/b"}, day, 100, today)
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "60 seconds") || !strings.Contains(err.Error(), "Ctrl-C") {
		t.Fatalf("error = %v", err)
	}
}

func TestThousands(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 6066: "6,066", 1234567: "1,234,567"} {
		if got := thousands(n); got != want {
			t.Errorf("thousands(%d) = %q, want %q", n, got, want)
		}
	}
}
