package source

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func makeOpenBotDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "openbot.db")
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE agent_usage_activity (
		agent_id TEXT NOT NULL, activity_id TEXT NOT NULL, session_id TEXT NOT NULL,
		turn_id TEXT NOT NULL, provider TEXT NOT NULL, model TEXT NOT NULL,
		kind TEXT NOT NULL, occurred_at TEXT NOT NULL,
		PRIMARY KEY(agent_id, activity_id))`)
	if err != nil {
		t.Fatal(err)
	}
	insert := func(agent, prov, model, kind, at string) {
		_, err := db.Exec(`INSERT INTO agent_usage_activity
		(agent_id, activity_id, session_id, turn_id, provider, model, kind, occurred_at)
		VALUES (?,?,?,?,?,?,?,?)`, agent, agent+"-"+kind+"-"+at, "s1", "t1", prov, model, kind, at)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert("agent-1", "cursor", "model-gamma[reasoning=high]", "assistant", "2026-11-18T08:23:37.248Z")
	insert("agent-2", "pi", "vendor/model-gamma", "user", "2026-11-17T07:00:00.000Z")
	insert("agent-3", "opencode", "vendor/model-delta", "turn", "2026-11-16T07:00:00.000Z")
	return path
}

func TestOpenBot_FetchSessions(t *testing.T) {
	ob, err := NewOpenBot(makeOpenBotDB(t))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ob.FetchSessions(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	first := rows[0]
	if first.AgentID != "agent-1" || first.Provider != "cursor" || first.Kind != "assistant" {
		t.Errorf("newest-first violated: %+v", first)
	}
	want := time.Date(2026, 11, 18, 8, 23, 37, 248000000, time.UTC)
	if !first.OccurredAt.Equal(want) {
		t.Errorf("OccurredAt=%v want %v", first.OccurredAt, want)
	}
	if ob.Name() != "openbot" {
		t.Errorf("Name=%q", ob.Name())
	}
}

func TestOpenBot_MissingFile(t *testing.T) {
	_, err := NewOpenBot(filepath.Join(t.TempDir(), "nope.db"))
	if err == nil || !strings.Contains(err.Error(), "nope.db") {
		t.Fatalf("want path in error, got %v", err)
	}
}

func TestOpenBot_NoTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.db")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close() // empty file: opens fine, but lacks the activity table
	ob, err := NewOpenBot(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ob.FetchSessions(context.Background(), 10)
	if err == nil || !strings.Contains(err.Error(), "agent_usage_activity") {
		t.Fatalf("want table name in error, got %v", err)
	}
}
