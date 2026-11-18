package agg

import (
	"math"
	"testing"
	"time"

	"github.com/ariksasmita/cursor-tracker/internal/source"
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

func TestComputeAggregate_DesignMetrics(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	p := basePeriod(now) // 30-day period, 10 days in, $30 of $100
	a := ComputeAggregate(now, &p)

	if a.DaysInPeriod != 30 {
		t.Errorf("DaysInPeriod=%d want 30", a.DaysInPeriod)
	}
	if a.DayOfPeriod != 11 {
		t.Errorf("DayOfPeriod=%d want 11", a.DayOfPeriod)
	}
	if math.Abs(a.ElapsedFraction-(10.0/30.0)) > 0.005 {
		t.Errorf("ElapsedFraction=%v want ~0.333", a.ElapsedFraction)
	}
	if math.Abs(a.SpendFraction-0.30) > 0.0001 {
		t.Errorf("SpendFraction=%v want 0.30", a.SpendFraction)
	}
	// pace = spend share / elapsed share = 0.30 / 0.333 = 0.90
	if math.Abs(a.PaceRatio-0.90) > 0.01 {
		t.Errorf("PaceRatio=%v want ~0.90", a.PaceRatio)
	}
	if a.RemainingCents != 7000 {
		t.Errorf("RemainingCents=%d want 7000", a.RemainingCents)
	}
	if math.Abs(a.ProjectedFraction-0.90) > 0.01 {
		t.Errorf("ProjectedFraction=%v want ~0.90", a.ProjectedFraction)
	}
}

func TestComputeAggregate_OverBudget(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	p := basePeriod(now)
	p.TotalSpendCents = 15000 // $150 of $100
	a := ComputeAggregate(now, &p)
	if a.RemainingCents != -5000 {
		t.Errorf("RemainingCents=%d want -5000", a.RemainingCents)
	}
	if a.PaceRatio <= 1 {
		t.Errorf("PaceRatio=%v want >1 when over pace", a.PaceRatio)
	}
}
