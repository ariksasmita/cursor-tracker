package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sasmitai/cursor-tracker/internal/history"
	"github.com/sasmitai/cursor-tracker/internal/source"
)

func goodResult(now time.Time) FetchResult {
	day := time.Date(2026, 11, 17, 0, 0, 0, 0, time.Local)
	return FetchResult{
		Period: &source.PeriodUsage{
			PeriodStartMs:   now.AddDate(0, 0, -17).UnixMilli(),
			PeriodEndMs:     now.AddDate(0, 0, 13).UnixMilli(),
			TotalSpendCents: 1290,
			LimitCents:      10000,
			FetchedAt:       now,
		},
		Daily: []source.DailySpend{
			{Day: day, Category: "model-alpha", SpendCents: 330, TotalTokens: 1234567},
			{Day: day, Category: "model-beta", SpendCents: 87, TotalTokens: 900000},
		},
		Sessions: []source.SessionEvent{
			{AgentID: "agent-1", Provider: "cursor", Model: "model-gamma", Kind: "assistant", OccurredAt: now.Add(-time.Hour)},
		},
		SourceName: "live",
		FetchedAt:  now,
	}
}

func newTestApp(t *testing.T, result FetchResult, fetchErr error, sessions []source.SessionEvent, sessErr error) *App {
	t.Helper()
	store, err := history.Open(t.TempDir() + "/h/history.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	deps := Deps{
		Primary: nil, Fallback: nil, Sessions: nil,
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local) },
	}
	res := result
	res.Sessions = sessions
	res.SessionsErr = sessErr
	app := New(deps)
	app.Fetch = func(ctx context.Context) (FetchResult, error) { return res, fetchErr }
	app.loadResult(res) // simulate a completed fetch without running tea
	return app
}

func TestOverviewRender(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	app := newTestApp(t, goodResult(now), nil, nil, nil)
	view := app.View()
	for _, want := range []string{"$12.90", "$100.00", "via live"} {
		if !strings.Contains(view, want) {
			t.Errorf("overview missing %q", want)
		}
	}
}

func TestWarnings_PrimaryFails(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	fallback := goodResult(now)
	fallback.SourceName = "snapshot"
	fallback.PrimaryError = errors.New("HTTP 500")
	app := newTestApp(t, fallback, nil, nil, nil)
	view := app.View()
	if !strings.Contains(view, "live") || !strings.Contains(view, "unavailable") {
		t.Errorf("warning line missing: %q", firstLine(view, 5))
	}
	if !strings.Contains(view, "$12.90") {
		t.Error("fallback data must still render")
	}
}

func TestEmptyHistory_NoProjection(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	app := newTestApp(t, goodResult(now), nil, nil, nil)
	// period started 17 days ago, so projection exists; force first-day case:
	res := goodResult(now)
	res.Period.PeriodStartMs = now.UnixMilli() // elapsed 0
	app.loadResult(res)
	view := app.View()
	if !strings.Contains(view, "—") {
		t.Errorf("no-projection must render an em dash, got: %q", firstLine(view, 6))
	}
}

func TestTabSwitch(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	app := newTestApp(t, goodResult(now), nil, []source.SessionEvent{
		{AgentID: "agent-9", Provider: "cursor", Model: "model-gamma", Kind: "turn", OccurredAt: now},
	}, nil)
	app.switchTab(3)
	view := app.View()
	if !strings.Contains(view, "Sessions") {
		t.Error("sessions tab header missing")
	}
	if !strings.Contains(view, "agent-9") {
		t.Error("session row missing")
	}
}

func TestSessionsError_TabStillRenders(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	app := newTestApp(t, goodResult(now), nil, nil, errors.New("db locked"))
	app.switchTab(3)
	view := app.View()
	if !strings.Contains(view, "Sessions") || !strings.Contains(view, "db locked") {
		t.Errorf("sessions tab must surface error, got %q", firstLine(view, 6))
	}
}

func TestModelsTab(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	app := newTestApp(t, goodResult(now), nil, nil, nil)
	app.switchTab(2)
	view := app.View()
	if !strings.Contains(view, "model-alpha") || !strings.Contains(view, "$4.17") {
		t.Errorf("models tab missing category/period total: %q", firstLine(view, 8))
	}
}

func firstLine(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, " | ")
}

func TestInitialFrame_ShowsLoading(t *testing.T) {
	app := newTestApp(t, FetchResult{}, nil, nil, nil)
	app.result = FetchResult{} // no fetch completed yet
	app.warning = ""
	if view := app.View(); !strings.Contains(view, "loading") {
		t.Errorf("pre-fetch frame must say loading, got: %q", firstLine(view, 4))
	}
}

func TestTickTriggersRefetch(t *testing.T) {
	app := newTestApp(t, goodResult(time.Now()), nil, nil, nil)
	calls := 0
	app.Fetch = func(ctx context.Context) (FetchResult, error) {
		calls++
		return FetchResult{}, nil
	}
	oldTick := tick
	tick = func(d time.Duration) tea.Cmd { return func() tea.Msg { return nil } }
	defer func() { tick = oldTick }()
	_, cmd := app.Update(tickMsg(time.Now()))
	if cmd == nil {
		t.Fatal("tick must return a command")
	}
	// Batch only returns the sub-commands; run them.
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				_ = c()
			}
		}
	}
	if calls == 0 {
		t.Fatal("auto-refresh tick must trigger a refetch")
	}
}
