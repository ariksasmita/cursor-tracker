// Package history persists observed spend snapshots into the tracker's own
// SQLite DB so trends accumulate beyond what Cursor serves per query.
package history

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sasmitai/cursor-tracker/internal/source"

	_ "modernc.org/sqlite"
)

var local = time.Local

func timeFromMs(ms int64) time.Time { return time.UnixMilli(ms).In(local) }

// Store is the tracker's own history database.
type Store struct {
	db *sql.DB
}

// Open creates (if needed) and opens the history DB at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS period_snapshots (
	fetched_at INTEGER PRIMARY KEY,
	period_start_ms INTEGER NOT NULL,
	period_end_ms INTEGER NOT NULL,
	total_spend_cents INTEGER NOT NULL,
	limit_cents INTEGER NOT NULL,
	included_spend_cents INTEGER NOT NULL,
	api_spend_cents INTEGER NOT NULL,
	auto_spend_cents INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS daily_spend (
	period_start_ms INTEGER NOT NULL,
	day TEXT NOT NULL,
	category TEXT NOT NULL,
	spend_cents INTEGER NOT NULL,
	total_tokens INTEGER NOT NULL,
	PRIMARY KEY (period_start_ms, day, category)
);`)
	return err
}

// Close releases the DB handle.
func (s *Store) Close() error { return s.db.Close() }

// UpsertPeriod stores a period snapshot keyed by FetchedAt. A poll whose
// spend and limit match the period's most recent snapshot is skipped, so one
// row is kept per observed spend level rather than one per refresh.
func (s *Store) UpsertPeriod(p source.PeriodUsage) error {
	var lastSpend, lastLimit int64
	err := s.db.QueryRow(`SELECT total_spend_cents, limit_cents FROM period_snapshots
WHERE period_start_ms = ? ORDER BY fetched_at DESC LIMIT 1`, p.PeriodStartMs).Scan(&lastSpend, &lastLimit)
	if err == nil && lastSpend == p.TotalSpendCents && lastLimit == p.LimitCents {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = s.db.Exec(`INSERT OR REPLACE INTO period_snapshots
(fetched_at, period_start_ms, period_end_ms, total_spend_cents, limit_cents, included_spend_cents, api_spend_cents, auto_spend_cents)
VALUES (?,?,?,?,?,?,?,?)`,
		p.FetchedAt.UnixMilli(), p.PeriodStartMs, p.PeriodEndMs, p.TotalSpendCents,
		p.LimitCents, p.IncludedSpendCents, p.ApiSpendCents, p.AutoSpendCents)
	return err
}

// UpsertDaily stores the daily rows observed for one billing period.
func (s *Store) UpsertDaily(periodStartMs int64, rows []source.DailySpend) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO daily_spend
(period_start_ms, day, category, spend_cents, total_tokens) VALUES (?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(periodStartMs, r.Day.Format("2006-01-02"), r.Category, r.SpendCents, r.TotalTokens); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Periods returns stored period snapshots ordered by fetch time.
func (s *Store) Periods() ([]source.PeriodUsage, error) {
	rows, err := s.db.Query(`SELECT fetched_at, period_start_ms, period_end_ms,
total_spend_cents, limit_cents, included_spend_cents, api_spend_cents, auto_spend_cents
FROM period_snapshots ORDER BY fetched_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []source.PeriodUsage
	for rows.Next() {
		var p source.PeriodUsage
		var fetchedMs int64
		if err := rows.Scan(&fetchedMs, &p.PeriodStartMs, &p.PeriodEndMs, &p.TotalSpendCents,
			&p.LimitCents, &p.IncludedSpendCents, &p.ApiSpendCents, &p.AutoSpendCents); err != nil {
			return nil, err
		}
		p.FetchedAt = timeFromMs(fetchedMs)
		out = append(out, p)
	}
	return out, rows.Err()
}

// DailyForPeriod returns stored daily rows for one billing period.
func (s *Store) DailyForPeriod(periodStartMs int64) ([]source.DailySpend, error) {
	rows, err := s.db.Query(`SELECT day, category, spend_cents, total_tokens
FROM daily_spend WHERE period_start_ms = ? ORDER BY day, category`, periodStartMs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []source.DailySpend
	for rows.Next() {
		var day string
		var r source.DailySpend
		if err := rows.Scan(&day, &r.Category, &r.SpendCents, &r.TotalTokens); err != nil {
			return nil, err
		}
		d, err := time.ParseInLocation("2006-01-02", day, local)
		if err != nil {
			return nil, fmt.Errorf("bad stored day %q: %w", day, err)
		}
		r.Day = d
		out = append(out, r)
	}
	return out, rows.Err()
}
