package source

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// OpenBot reads agent session activity from OpenBot's openbot.db.
type OpenBot struct {
	db *sql.DB
}

// NewOpenBot opens OpenBot's DB read-only with a short busy timeout so a
// running OpenBot app doesn't block reads.
func NewOpenBot(path string) (*OpenBot, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_busy_timeout=2000")
	if err != nil {
		return nil, fmt.Errorf("open openbot db %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open openbot db %s: %w", path, err)
	}
	return &OpenBot{db: db}, nil
}

// Name implements SessionSource.
func (o *OpenBot) Name() string { return "openbot" }

// FetchSessions returns the newest agent activity rows.
func (o *OpenBot) FetchSessions(ctx context.Context, limit int) ([]SessionEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := o.db.QueryContext(ctx, `SELECT agent_id, provider, model, kind, occurred_at
FROM agent_usage_activity ORDER BY occurred_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query agent_usage_activity: %w", err)
	}
	defer rows.Close()
	var out []SessionEvent
	for rows.Next() {
		var e SessionEvent
		var at string
		if err := rows.Scan(&e.AgentID, &e.Provider, &e.Model, &e.Kind, &at); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return nil, fmt.Errorf("bad occurred_at %q: %w", at, err)
		}
		e.OccurredAt = t
		out = append(out, e)
	}
	return out, rows.Err()
}
