package tui

import (
	"context"
	"fmt"
)

// defaultFetch builds the production Fetch from deps: primary source first,
// fallback on failure, sessions best-effort.
func defaultFetch(deps Deps) func(ctx context.Context) (FetchResult, error) {
	return func(ctx context.Context) (FetchResult, error) {
		var res FetchResult
		if deps.Primary == nil && deps.Fallback == nil {
			return res, fmt.Errorf("no data source available")
		}
		use := deps.Primary
		if use == nil {
			use = deps.Fallback
		}
		p, perr := use.FetchPeriodUsage(ctx)
		if perr != nil && deps.Fallback != nil && use != deps.Fallback {
			var ferr error
			p, ferr = deps.Fallback.FetchPeriodUsage(ctx)
			if ferr != nil {
				return res, fmt.Errorf("primary: %v; fallback: %w", perr, ferr)
			}
			res.SourceName = deps.Fallback.Name()
			res.PrimaryError = perr
		} else if perr != nil {
			return res, perr
		} else {
			res.SourceName = use.Name()
		}
		res.Period = p

		dailySource := use
		if res.PrimaryError != nil {
			dailySource = deps.Fallback
		}
		res.Daily, _ = dailySource.FetchDailySpend(ctx, p.PeriodStartMs, p.PeriodEndMs)
		res.FetchedAt = deps.Now()

		if deps.Sessions != nil {
			res.Sessions, res.SessionsErr = deps.Sessions.FetchSessions(ctx, 50)
		}
		return res, nil
	}
}
