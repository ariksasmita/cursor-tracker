// Package agg derives burn-rate and projection metrics from a PeriodUsage.
package agg

import (
	"time"

	"github.com/sasmitai/cursor-tracker/internal/source"
)

// Aggregate holds derived period metrics. Zero values plus Has* flags
// express "not computable" (see each field).
type Aggregate struct {
	BurnRateCentsPerDay     float64
	ProjectedEomSpendCents  float64 // 0 + HasProjection=false when not computable
	DaysElapsed, DaysRemaining float64
	DaysToLimit             float64 // 0 when not computable
	HasLimit, HasProjection bool
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
			}
		}
	}
	return a
}
