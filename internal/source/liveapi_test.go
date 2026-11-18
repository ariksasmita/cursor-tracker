package source

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func stubPost(bodyByURL map[string][]byte, statusByURL map[string]int) (reset func(), calls *[]string) {
	old := post
	callsRec := &[]string{}
	calls = callsRec
	post = func(url string, body []byte, token string) ([]byte, int, error) {
		*callsRec = append(*callsRec, url)
		if code, ok := statusByURL[url]; ok {
			return []byte(`{"error":"unauthorized"}`), code, nil
		}
		b, ok := bodyByURL[url]
		if !ok {
			return nil, 0, errors.New("unexpected url " + url)
		}
		return b, 200, nil
	}
	return func() { post = old }, calls
}

func TestFetchPeriodUsage(t *testing.T) {
	daily := loadFixture(t, "daily.json")
	reset, _ := stubPost(map[string][]byte{
		"https://api2.cursor.sh/aiserver.v1.DashboardService/GetDailySpendByCategory": daily,
	}, nil)
	defer reset()

	api := NewLiveAPI("https://api2.cursor.sh", "tok")
	p, err := api.FetchPeriodUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var wantSum int64
	// sum computed from fixture independently in test via the exported DailySpend path
	d, err := api.FetchDailySpend(context.Background(), p.PeriodStartMs, p.PeriodEndMs)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range d {
		wantSum += r.SpendCents
	}
	if p.TotalSpendCents != wantSum {
		t.Errorf("TotalSpendCents=%d want %d", p.TotalSpendCents, wantSum)
	}
	if p.LimitCents != 10000 {
		t.Errorf("LimitCents=%d want 10000", p.LimitCents)
	}
	if p.PeriodEndMs <= p.PeriodStartMs {
		t.Errorf("period bounds not sane: %d..%d", p.PeriodStartMs, p.PeriodEndMs)
	}
	if start := time.UnixMilli(p.PeriodStartMs).In(time.Local); start.Day() != 1 {
		t.Errorf("period must start on the 1st, got %v", start)
	}
	if p.FetchedAt.IsZero() {
		t.Error("FetchedAt not set")
	}
}

func TestFetchDailySpend(t *testing.T) {
	reset, _ := stubPost(map[string][]byte{
		"https://api2.cursor.sh/aiserver.v1.DashboardService/GetDailySpendByCategory": loadFixture(t, "daily.json"),
	}, nil)
	defer reset()

	api := NewLiveAPI("https://api2.cursor.sh", "tok")
	rows, err := api.FetchDailySpend(context.Background(), 1793577600000, 1796083200000)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 1 {
		t.Fatal("expected rows")
	}
	var found bool
	for _, r := range rows {
		if r.Category == "model-alpha" && r.SpendCents == 330 && r.TotalTokens == 1234567 {
			day := time.Date(2026, 11, 3, 0, 0, 0, 0, time.Local)
			if !r.Day.Equal(day) {
				t.Errorf("Day=%v want %v", r.Day, day)
			}
			found = true
		}
	}
	if !found {
		t.Error("fixture row model-alpha not mapped")
	}
}

func TestAPIError(t *testing.T) {
	reset, _ := stubPost(nil, map[string]int{
		"https://api2.cursor.sh" + methodDailySpend: 401,
	})
	defer reset()

	api := NewLiveAPI("https://api2.cursor.sh", "tok")
	_, err := api.FetchPeriodUsage(context.Background())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want 401 error, got %v", err)
	}
}

func TestName(t *testing.T) {
	api := NewLiveAPI("https://api2.cursor.sh", "tok")
	if api.Name() != "live" {
		t.Errorf("Name=%q want live", api.Name())
	}
}

func TestAuthorizationHeaderSent(t *testing.T) {
	old := post
	defer func() { post = old }()
	var gotToken string
	post = func(url string, body []byte, token string) ([]byte, int, error) {
		gotToken = token
		return loadFixture(t, "daily.json"), 200, nil
	}
	api := NewLiveAPI("https://api2.cursor.sh", "tok-secret")
	if _, err := api.FetchPeriodUsage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotToken != "tok-secret" {
		t.Fatalf("token not passed to transport: %q", gotToken)
	}
}

func TestFetchPeriodUsage_UsesCalendarMonth(t *testing.T) {
	oldNow := nowFn
	nowFn = func() time.Time { return time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local) }
	defer func() { nowFn = oldNow }()

	var sentStart, sentEnd int64
	old := post
	defer func() { post = old }()
	post = func(url string, body []byte, token string) ([]byte, int, error) {
		var req struct {
			PeriodStartMs int64 `json:"periodStartMs"`
			PeriodEndMs   int64 `json:"periodEndMs"`
		}
		_ = json.Unmarshal(body, &req)
		sentStart, sentEnd = req.PeriodStartMs, req.PeriodEndMs
		return loadFixture(t, "daily.json"), 200, nil
	}

	api := NewLiveAPI("https://api2.cursor.sh", "tok")
	p, err := api.FetchPeriodUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantStart := time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	wantEnd := time.Date(2026, 12, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	if p.PeriodStartMs != wantStart || p.PeriodEndMs != wantEnd {
		t.Fatalf("period must be calendar month: got %d..%d want %d..%d",
			p.PeriodStartMs, p.PeriodEndMs, wantStart, wantEnd)
	}
	if sentStart != wantStart || sentEnd != wantEnd {
		t.Fatalf("request must ask for the month window: sent %d..%d", sentStart, sentEnd)
	}
}
