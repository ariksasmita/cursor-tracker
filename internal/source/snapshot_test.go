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
	ls, err := NewLocalSnapshot(makeSnapshotDB(t, snapshotJSON, true))
	if err != nil {
		t.Fatal(err)
	}
	p, err := ls.FetchPeriodUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.PeriodStartMs != 1793577600000 || p.PeriodEndMs != 1796083200000 {
		t.Errorf("bounds: %d..%d", p.PeriodStartMs, p.PeriodEndMs)
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
