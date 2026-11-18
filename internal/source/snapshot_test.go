package source

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const snapshotJSON = `{
 "version":2,
 "periodStartMs":1793577600000,
 "periodEndMs":1796083200000,
 "fetchedAt":1794560969745,
 "charts":[
  {"periodStartMs":1793577600000,"periodEndMs":1796083200000,"spendType":"all","fetchedAt":1794560972165,
   "response":"{\"dailySpend\":[{\"day\":\"1793664000000\",\"category\":\"model-alpha\",\"spendCents\":330,\"totalTokens\":\"1234567\"},{\"day\":\"1793836800000\",\"category\":\"model-alpha\",\"spendCents\":125,\"totalTokens\":\"45678\"}],\"categories\":[\"model-alpha\"],\"effectiveLimitCents\":10000}"},
  {"periodStartMs":1790899200000,"periodEndMs":1793491200000,"spendType":"all","fetchedAt":1794560975037,
   "response":"{\"dailySpend\":[{\"day\":\"1791244800000\",\"category\":\"default\",\"spendCents\":234,\"totalTokens\":\"1234567\"}],\"categories\":[\"default\"],\"effectiveLimitCents\":9000}"}
 ]}`

func makeSnapshotDB(t *testing.T, value string, withRow bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.vscdb")
	f, _ := os.Create(path)
	f.Close()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT)"); err != nil {
		t.Fatal(err)
	}
	if withRow {
		if _, err := db.Exec("INSERT INTO ItemTable (key, value) VALUES ('aiSettings.enterprisePlanUsageSnapshot', ?)", value); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestLocalSnapshot_PeriodFromNewestChart(t *testing.T) {
	oldNow := nowFn
	nowFn = func() time.Time { return time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local) }
	defer func() { nowFn = oldNow }()

	ls, err := NewLocalSnapshot(makeSnapshotDB(t, snapshotJSON, true))
	if err != nil {
		t.Fatal(err)
	}
	p, err := ls.FetchPeriodUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.PeriodStartMs != time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local).UnixMilli() {
		t.Errorf("bounds must be the calendar month, got %d..%d", p.PeriodStartMs, p.PeriodEndMs)
	}
	if p.TotalSpendCents != 455 { // 330+125 from newest chart
		t.Errorf("TotalSpendCents=%d want 455", p.TotalSpendCents)
	}
	if p.LimitCents != 10000 {
		t.Errorf("LimitCents=%d want 10000", p.LimitCents)
	}
	if !p.FetchedAt.Equal(time.UnixMilli(1794560969745)) {
		t.Errorf("FetchedAt=%v", p.FetchedAt)
	}
	if ls.Name() != "snapshot" {
		t.Errorf("Name=%q", ls.Name())
	}
}

func TestLocalSnapshot_DailyFilteredByPeriod(t *testing.T) {
	ls, _ := NewLocalSnapshot(makeSnapshotDB(t, snapshotJSON, true))
	rows, err := ls.FetchDailySpend(context.Background(), 1790899200000, 1793491200000)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Category != "default" || rows[0].SpendCents != 234 {
		t.Fatalf("want older period single row, got %+v", rows)
	}
}

func TestLocalSnapshot_KeyAbsent(t *testing.T) {
	ls, err := NewLocalSnapshot(makeSnapshotDB(t, "", false))
	if err != nil {
		t.Fatal(err)
	}
	_, err = ls.FetchPeriodUsage(context.Background())
	if err == nil || !strings.Contains(err.Error(), "snapshot not found") {
		t.Fatalf("want snapshot-not-found error, got %v", err)
	}
}

func TestLocalSnapshot_DailyResolvesForReportedBounds(t *testing.T) {
	// The reported period bounds must resolve when handed back to
	// FetchDailySpend (the TUI does exactly that).
	oldNow := nowFn
	nowFn = func() time.Time { return time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local) }
	defer func() { nowFn = oldNow }()

	ls, err := NewLocalSnapshot(makeSnapshotDB(t, snapshotJSON, true))
	if err != nil {
		t.Fatal(err)
	}
	p, err := ls.FetchPeriodUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ls.FetchDailySpend(context.Background(), p.PeriodStartMs, p.PeriodEndMs)
	if err != nil || len(rows) == 0 {
		t.Fatalf("daily rows must resolve for reported bounds: %d rows, err %v", len(rows), err)
	}
	var sum int64
	for _, r := range rows {
		sum += r.SpendCents
	}
	if sum != p.TotalSpendCents {
		t.Fatalf("daily sum %d must equal period total %d", sum, p.TotalSpendCents)
	}
}

func TestLocalSnapshot_MonthWindow(t *testing.T) {
	oldNow := nowFn
	nowFn = func() time.Time { return time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local) }
	defer func() { nowFn = oldNow }()

	ls, err := NewLocalSnapshot(makeSnapshotDB(t, snapshotJSON, true))
	if err != nil {
		t.Fatal(err)
	}
	p, err := ls.FetchPeriodUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantStart := time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	wantEnd := time.Date(2026, 12, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	if p.PeriodStartMs != wantStart || p.PeriodEndMs != wantEnd {
		t.Fatalf("snapshot must also use the calendar month: %d..%d", p.PeriodStartMs, p.PeriodEndMs)
	}
	// Only the November rows count: 330 + 125, not the October row.
	if p.TotalSpendCents != 455 {
		t.Fatalf("month total=%d want 455", p.TotalSpendCents)
	}
	if p.LimitCents != 10000 {
		t.Errorf("limit=%d want 10000", p.LimitCents)
	}
	rows, err := ls.FetchDailySpend(context.Background(), p.PeriodStartMs, p.PeriodEndMs)
	if err != nil || len(rows) != 2 {
		t.Fatalf("want 2 November rows, got %d (err %v)", len(rows), err)
	}
}
