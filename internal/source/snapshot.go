package source

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
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

func (s *LocalSnapshot) chartFor(startMs, endMs int64) (*chartResponse, error) {
	doc, err := s.load()
	if err != nil {
		return nil, err
	}
	charts := append([]struct {
		PeriodStartMs int64  `json:"periodStartMs"`
		PeriodEndMs   int64  `json:"periodEndMs"`
		Response      string `json:"response"`
	}{}, doc.Charts...)
	sort.SliceStable(charts, func(i, j int) bool { return charts[i].PeriodStartMs > charts[j].PeriodStartMs })
	for _, c := range charts {
		if c.PeriodStartMs == startMs && c.PeriodEndMs == endMs {
			var out chartResponse
			if err := json.Unmarshal([]byte(c.Response), &out); err != nil {
				return nil, fmt.Errorf("parse chart response: %w", err)
			}
			return &out, nil
		}
	}
	// Fall back to newest chart when exact bounds not requested.
	if len(charts) > 0 && startMs == 0 && endMs == 0 {
		var out chartResponse
		if err := json.Unmarshal([]byte(charts[0].Response), &out); err != nil {
			return nil, fmt.Errorf("parse chart response: %w", err)
		}
		return &out, nil
	}
	return &chartResponse{}, nil
}

// FetchPeriodUsage implements UsageSource from the newest cached period.
func (s *LocalSnapshot) FetchPeriodUsage(ctx context.Context) (*PeriodUsage, error) {
	doc, err := s.load()
	if err != nil {
		return nil, err
	}
	chart, err := s.chartFor(0, 0)
	if err != nil {
		return nil, err
	}
	var total int64
	for _, r := range chart.DailySpend {
		total += r.SpendCents
	}
	return &PeriodUsage{
		PeriodStartMs:   doc.PeriodStartMs,
		PeriodEndMs:     doc.PeriodEndMs,
		TotalSpendCents: total,
		LimitCents:      chart.EffectiveLimitCents,
		FetchedAt:       time.UnixMilli(doc.FetchedAt),
	}, nil
}

// FetchDailySpend implements UsageSource, returning the cached rows for the
// requested period bounds.
func (s *LocalSnapshot) FetchDailySpend(ctx context.Context, periodStartMs, periodEndMs int64) ([]DailySpend, error) {
	chart, err := s.chartFor(periodStartMs, periodEndMs)
	if err != nil {
		return nil, err
	}
	rows := make([]DailySpend, 0, len(chart.DailySpend))
	for _, r := range chart.DailySpend {
		dayMs, err := strconv.ParseInt(r.Day, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bad day %q: %w", r.Day, err)
		}
		tokens, _ := strconv.ParseInt(r.TotalTokens, 10, 64)
		utc := time.UnixMilli(dayMs).UTC()
		rows = append(rows, DailySpend{
			Day:         time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.Local),
			Category:    r.Category,
			SpendCents:  r.SpendCents,
			TotalTokens: tokens,
		})
	}
	return rows, nil
}
