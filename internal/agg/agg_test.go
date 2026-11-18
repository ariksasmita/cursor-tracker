package agg

import (
	"math"
	"testing"
	"time"

	"github.com/sasmitai/cursor-tracker/internal/source"
)

func ptr(p source.PeriodUsage) *source.PeriodUsage { return &p }

// 30-day period started 10 days ago, $30.00 spent, $100.00 limit.
func basePeriod(now time.Time) source.PeriodUsage {
	start := now.AddDate(0, 0, -10)
	end := now.AddDate(0, 0, 20)
	return source.PeriodUsage{
		PeriodStartMs:   start.UnixMilli(),
		PeriodEndMs:     end.UnixMilli(),
		TotalSpendCents: 3000,
		LimitCents:      10000,
		FetchedAt:       now,
	}
}

func TestComputeAggregate_Normal(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	a := ComputeAggregate(now, ptr(basePeriod(now)))
	if math.Abs(a.BurnRateCentsPerDay-300) > 0.01 {
		t.Errorf("burn=%v want 300", a.BurnRateCentsPerDay)
	}
	if !a.HasLimit || !a.HasProjection {
		t.Errorf("flags: %+v", a)
	}
	if math.Abs(a.ProjectedEomSpendCents-9000) > 0.01 {
		t.Errorf("projection=%v want 9000", a.ProjectedEomSpendCents)
	}
	if math.Abs(a.DaysToLimit-(7000.0/300.0)) > 0.01 {
		t.Errorf("daysToLimit=%v want %v", a.DaysToLimit, 7000.0/300.0)
	}
	if a.DaysElapsed < 9.9 || a.DaysElapsed > 10.1 || a.DaysRemaining < 19.9 || a.DaysRemaining > 20.1 {
		t.Errorf("elapsed=%v remaining=%v", a.DaysElapsed, a.DaysRemaining)
	}
}

func TestComputeAggregate_FirstDay(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	p := source.PeriodUsage{PeriodStartMs: now.UnixMilli(), PeriodEndMs: now.AddDate(0, 0, 30).UnixMilli(), TotalSpendCents: 500, LimitCents: 10000}
	a := ComputeAggregate(now, &p)
	if a.BurnRateCentsPerDay != 0 || a.HasProjection {
		t.Errorf("elapsed<=1 day must yield no projection: %+v", a)
	}
}

func TestComputeAggregate_NoLimit(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	p := basePeriod(now)
	p.LimitCents = 0
	a := ComputeAggregate(now, &p)
	if a.HasLimit || a.DaysToLimit != 0 {
		t.Errorf("limit 0 must disable limit features: %+v", a)
	}
	if a.BurnRateCentsPerDay == 0 {
		t.Error("burn rate still computed without limit")
	}
}

func TestComputeAggregate_ZeroBurn(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	p := basePeriod(now)
	p.TotalSpendCents = 0
	a := ComputeAggregate(now, &p)
	if !a.HasLimit || a.HasProjection || a.DaysToLimit != 0 {
		t.Errorf("zero burn with limit: limit yes, projection/daysToLimit no: %+v", a)
	}
}
