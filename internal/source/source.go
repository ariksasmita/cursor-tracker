// Package source defines the data contracts shared by all cursor-tracker
// data sources (live API, OpenBot DB, local snapshot fallback).
package source

import (
	"context"
	"time"
)

// PeriodUsage is a billing-period spend summary against the plan limit.
// All money fields are US cents.
type PeriodUsage struct {
	PeriodStartMs, PeriodEndMs                        int64
	TotalSpendCents, LimitCents                       int64
	IncludedSpendCents, ApiSpendCents, AutoSpendCents int64
	FetchedAt                                         time.Time
}

// DailySpend is one day's spend for one model category.
type DailySpend struct {
	Day         time.Time // parsed from epoch-ms "day" field, local tz
	Category    string
	SpendCents  int64
	TotalTokens int64
}

// SessionEvent is one agent-usage activity row (user/assistant/turn).
type SessionEvent struct {
	AgentID, Provider, Model, Kind string
	OccurredAt                     time.Time
}

// UsageSource provides plan spend data.
type UsageSource interface {
	Name() string
	FetchPeriodUsage(ctx context.Context) (*PeriodUsage, error)
	FetchDailySpend(ctx context.Context, periodStartMs, periodEndMs int64) ([]DailySpend, error)
}

// SessionSource provides agent session activity.
type SessionSource interface {
	FetchSessions(ctx context.Context, limit int) ([]SessionEvent, error)
}
