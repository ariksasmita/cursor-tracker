// Package agg derives burn-rate, pace, and projection metrics from a
// PeriodUsage.
package agg

import (
	"math"
	"time"

	"github.com/sasmitai/cursor-tracker/internal/source"
)

// Aggregate holds derived period metrics. Zero values plus Has* flags
// express "not computable" (see each field).
type Aggregate struct {
	BurnRateCentsPerDay        float64
	ProjectedEomSpendCents     float64 // 0 + HasProjection=false when not computable
	DaysElapsed, DaysRemaining float64
	DaysToLimit                float64 // 0 when not computable
	HasLimit, HasProjection    bool

	// Presentation metrics.
	DaysInPeriod      int     // whole days in the period, at least 1
	DayOfPeriod       int     // 1-based day of the period containing now
	ElapsedFraction   float64 // 0..1+: fraction of the period elapsed
	SpendFraction     float64 // spend / limit, 0 when no limit
	ProjectedFraction float64 // projection / limit, 0 when not computable
	PaceRatio         float64 // spend share / elapsed share; 1.0 = exactly on pace
	RemainingCents    int64   // limit - spend; negative when over budget
}

// ComputeAggregate derives metrics for the period containing now.
// Rules: elapsed <= 1 day → no projection; LimitCents <= 0 → no limit
// features; burn <= 0 → no projection and no days-to-limit.
func ComputeAggregate(now time.Time, p *source.PeriodUsage) Aggregate {
	a := Aggregate{HasLimit: p.LimitCents > 0}

	const dayMs = 24 * float64(time.Hour) / float64(time.Millisecond)
	start := float64(p.PeriodStartMs)
	end := float64(p.PeriodEndMs)
	nowMs := float64(now.UnixMilli())

	a.DaysElapsed = (nowMs - start) / dayMs
	a.DaysRemaining = (end - nowMs) / dayMs
	if a.DaysElapsed < 0 {
		a.DaysElapsed = 0
	}
	if a.DaysRemaining < 0 {
		a.DaysRemaining = 0
	}

	a.DaysInPeriod = int(math.Round((end - start) / dayMs))
	if a.DaysInPeriod < 1 {
		a.DaysInPeriod = 1
	}
	a.DayOfPeriod = int(math.Floor(a.DaysElapsed)) + 1
	if a.DayOfPeriod < 1 {
		a.DayOfPeriod = 1
	}
	if a.DayOfPeriod > a.DaysInPeriod {
		a.DayOfPeriod = a.DaysInPeriod
	}
	a.ElapsedFraction = a.DaysElapsed / float64(a.DaysInPeriod)

	if a.HasLimit {
		a.SpendFraction = float64(p.TotalSpendCents) / float64(p.LimitCents)
		a.RemainingCents = p.LimitCents - p.TotalSpendCents
		if a.ElapsedFraction > 0 {
			a.PaceRatio = a.SpendFraction / a.ElapsedFraction
		}
	}

	if a.DaysElapsed > 1 {
		a.BurnRateCentsPerDay = float64(p.TotalSpendCents) / a.DaysElapsed
		if a.BurnRateCentsPerDay > 0 {
			a.HasProjection = true
			a.ProjectedEomSpendCents = a.BurnRateCentsPerDay * (a.DaysElapsed + a.DaysRemaining)
			if a.HasLimit {
				a.DaysToLimit = float64(p.LimitCents-p.TotalSpendCents) / a.BurnRateCentsPerDay
				if a.DaysToLimit < 0 {
					a.DaysToLimit = 0
				}
				a.ProjectedFraction = a.ProjectedEomSpendCents / float64(p.LimitCents)
			}
		}
	}
	return a
}
