package source

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

const snapshotKey = "aiSettings.enterprisePlanUsageSnapshot"

// LocalSnapshot reads the Cursor app's cached usage dashboard snapshot from
// state.vscdb. Stale by nature (updated only when the app runs) — fallback.
type LocalSnapshot struct {
	path string
}

// NewLocalSnapshot points at a state.vscdb path.
func NewLocalSnapshot(path string) (*LocalSnapshot, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("state db %s: %w", path, err)
	}
	return &LocalSnapshot{path: path}, nil
}

// Name implements UsageSource.
func (s *LocalSnapshot) Name() string { return "snapshot" }

type snapshotDoc struct {
	PeriodStartMs int64 `json:"periodStartMs"`
	PeriodEndMs   int64 `json:"periodEndMs"`
	FetchedAt     int64 `json:"fetchedAt"`
	Charts        []struct {
		PeriodStartMs int64  `json:"periodStartMs"`
		PeriodEndMs   int64  `json:"periodEndMs"`
		Response      string `json:"response"`
	} `json:"charts"`
}

type chartResponse struct {
	DailySpend []struct {
		Day         string `json:"day"`
		Category    string `json:"category"`
		SpendCents  int64  `json:"spendCents"`
		TotalTokens string `json:"totalTokens"`
	} `json:"dailySpend"`
	EffectiveLimitCents int64 `json:"effectiveLimitCents"`
}

func (s *LocalSnapshot) load() (*snapshotDoc, error) {
	db, err := sql.Open("sqlite", "file:"+s.path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var raw string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key=?", snapshotKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("snapshot not found in %s (key %s)", s.path, snapshotKey)
	}
	if err != nil {
		return nil, err
	}
	var doc snapshotDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("parse snapshot: %w", err)
	}
	return &doc, nil
}

// FetchPeriodUsage implements UsageSource for the current calendar month,
// summing cached rows that fall inside it. Bounds are the month, not the
// cached chart's own range, so the tracker reports one consistent window
// whichever source answered.
func (s *LocalSnapshot) FetchPeriodUsage(ctx context.Context) (*PeriodUsage, error) {
	doc, err := s.load()
	if err != nil {
		return nil, err
	}
	startMs, endMs := monthBounds(nowFn())
	rows, err := s.FetchDailySpend(ctx, startMs, endMs)
	if err != nil {
		return nil, err
	}
	var total int64
	for _, r := range rows {
		total += r.SpendCents
	}
	return &PeriodUsage{
		PeriodStartMs:   startMs,
		PeriodEndMs:     endMs,
		TotalSpendCents: total,
		LimitCents:      s.limitCents(doc),
		FetchedAt:       time.UnixMilli(doc.FetchedAt),
	}, nil
}

// limitCents reports the newest cached chart's effective limit.
func (s *LocalSnapshot) limitCents(doc *snapshotDoc) int64 {
	if len(doc.Charts) == 0 {
		return 0
	}
	newest := doc.Charts[0]
	for _, c := range doc.Charts[1:] {
		if c.PeriodStartMs > newest.PeriodStartMs {
			newest = c
		}
	}
	var chart chartResponse
	if err := json.Unmarshal([]byte(newest.Response), &chart); err != nil {
		return 0
	}
	return chart.EffectiveLimitCents
}

// FetchDailySpend implements UsageSource, returning cached rows that fall
// inside [periodStartMs, periodEndMs) across every cached chart.
func (s *LocalSnapshot) FetchDailySpend(ctx context.Context, periodStartMs, periodEndMs int64) ([]DailySpend, error) {
	doc, err := s.load()
	if err != nil {
		return nil, err
	}
	startDay := time.UnixMilli(periodStartMs).In(time.Local)
	start := time.Date(startDay.Year(), startDay.Month(), startDay.Day(), 0, 0, 0, 0, time.Local)
	endDay := time.UnixMilli(periodEndMs).In(time.Local)
	end := time.Date(endDay.Year(), endDay.Month(), endDay.Day(), 0, 0, 0, 0, time.Local)

	seen := make(map[string]struct{}, len(doc.Charts))
	var rows []DailySpend
	for _, c := range doc.Charts {
		if _, dup := seen[c.Response]; dup {
			continue // the same chart can appear for multiple spend types
		}
		seen[c.Response] = struct{}{}
		var chart chartResponse
		if err := json.Unmarshal([]byte(c.Response), &chart); err != nil {
			return nil, fmt.Errorf("parse chart response: %w", err)
		}
		for _, r := range chart.DailySpend {
			dayMs, err := strconv.ParseInt(r.Day, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("bad day %q: %w", r.Day, err)
			}
			utc := time.UnixMilli(dayMs).UTC()
			day := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.Local)
			if day.Before(start) || !day.Before(end) {
				continue
			}
			tokens, _ := strconv.ParseInt(r.TotalTokens, 10, 64)
			rows = append(rows, DailySpend{
				Day:         day,
				Category:    r.Category,
				SpendCents:  r.SpendCents,
				TotalTokens: tokens,
			})
		}
	}
	return rows, nil
}
