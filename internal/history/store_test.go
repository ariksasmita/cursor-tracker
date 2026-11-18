package history

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sasmitai/cursor-tracker/internal/source"
)

func samplePeriod(fetched time.Time) source.PeriodUsage {
	return source.PeriodUsage{
		PeriodStartMs:     1793577600000,
		PeriodEndMs:       1796083200000,
		TotalSpendCents:   1290,
		LimitCents:        10000,
		IncludedSpendCents: 2000,
		ApiSpendCents:     300,
		AutoSpendCents:    990,
		FetchedAt:         fetched,
	}
}

func TestUpsertPeriod_Idempotent(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "h", "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ts := time.UnixMilli(1795000000000)
	if err := s.UpsertPeriod(samplePeriod(ts)); err != nil {
		t.Fatal(err)
	}
	p2 := samplePeriod(ts)
	p2.TotalSpendCents = 1500
	if err := s.UpsertPeriod(p2); err != nil {
		t.Fatal(err)
	}

	periods, err := s.Periods()
	if err != nil {
		t.Fatal(err)
	}
	if len(periods) != 1 {
		t.Fatalf("want 1 period, got %d", len(periods))
	}
	if periods[0].TotalSpendCents != 1500 {
		t.Errorf("want latest value 1500, got %d", periods[0].TotalSpendCents)
	}
}

func TestUpsertDaily_Idempotent(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	day := time.Date(2026, 11, 3, 0, 0, 0, 0, time.Local) // DailySpend.Day is local midnight (date semantics)
	rows := []source.DailySpend{{Day: day, Category: "m1", SpendCents: 330, TotalTokens: 1234567}}
	for i := 0; i < 2; i++ {
		if err := s.UpsertDaily(1793577600000, rows); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.DailyForPeriod(1793577600000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SpendCents != 330 {
		t.Fatalf("want 1 row spend 330, got %+v", got)
	}
	if !got[0].Day.Equal(day) {
		t.Errorf("Day=%v want %v", got[0].Day, day)
	}
}

func TestEmptyStore(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	periods, err := s.Periods()
	if err != nil || len(periods) != 0 {
		t.Fatalf("want empty no error, got %v %v", periods, err)
	}
	rows, err := s.DailyForPeriod(1)
	if err != nil || len(rows) != 0 {
		t.Fatalf("want empty no error, got %v %v", rows, err)
	}
}
