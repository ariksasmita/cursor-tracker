package tui

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ariksasmita/cursor-tracker/internal/agg"
	"github.com/ariksasmita/cursor-tracker/internal/history"
	"github.com/ariksasmita/cursor-tracker/internal/source"
)

func aggFor(a *App) agg.Aggregate {
	return agg.ComputeAggregate(a.deps.Now(), a.result.Period)
}

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
	if !strings.Contains(view, "AGENT ACTIVITY") {
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
	if !strings.Contains(view, "AGENT ACTIVITY") || !strings.Contains(view, "db locked") {
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

func TestMetricsRowMetricsOnOneLine(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	res := goodResult(now)
	res.Period.PeriodStartMs = time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	res.Period.PeriodEndMs = time.Date(2026, 12, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	app := newTestApp(t, res, nil, nil, nil)
	app.compact = false
	view := app.View()

	var labelLine string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "BURN RATE") {
			labelLine = line
		}
	}
	if labelLine == "" {
		t.Fatal("BURN RATE label missing")
	}
	for _, want := range []string{"PROJECTED EOM", "DAYS TO LIMIT", "ELAPSED"} {
		if !strings.Contains(labelLine, want) {
			t.Errorf("metrics must share one row; %q missing from %q", want, labelLine)
		}
	}
}

func TestDailyChartZeroFillsWholeMonth(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	res := goodResult(now)
	res.Period.PeriodStartMs = time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	res.Period.PeriodEndMs = time.Date(2026, 12, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	res.Daily = []source.DailySpend{
		{Day: time.Date(2026, 11, 16, 0, 0, 0, 0, time.Local), Category: "m1", SpendCents: 500},
		{Day: time.Date(2026, 11, 17, 0, 0, 0, 0, time.Local), Category: "m1", SpendCents: 600},
	}
	app := newTestApp(t, res, nil, nil, nil)
	days, _ := app.dailyTotals()
	if len(days) != 18 {
		t.Fatalf("chart must span day 1..18, got %d columns", len(days))
	}
	if days[0] != 0 || days[14] != 0 {
		t.Errorf("days without spend must be zero-filled, got %v", days)
	}
	if days[15] != 500 || days[16] != 600 {
		t.Errorf("known days wrong: %v", days)
	}
}

func TestOverBudgetLabel(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	res := goodResult(now)
	res.Period.TotalSpendCents = 15000 // over the $100 limit
	app := newTestApp(t, res, nil, nil, nil)
	view := app.View()
	if !strings.Contains(view, "OVER BUDGET") {
		t.Errorf("over-budget state must be labelled, got: %q", firstLine(view, 8))
	}
}

func TestSliceRows(t *testing.T) {
	rows := make([]string, 100)
	for i := range rows {
		rows[i] = fmt.Sprintf("row-%d", i)
	}
	got, more := sliceRows(rows, 0, 10)
	if len(got) != 10 || got[0] != "row-0" {
		t.Fatalf("first page wrong: %d %q", len(got), got[0])
	}
	if more != 90 {
		t.Errorf("more=%d want 90", more)
	}
	got, more = sliceRows(rows, 95, 10)
	if len(got) != 5 || got[0] != "row-95" || more != 0 {
		t.Fatalf("clamped page wrong: %d %q more=%d", len(got), got[0], more)
	}
	if got, _ := sliceRows(rows, 500, 10); len(got) != 0 {
		t.Errorf("offset past end must yield nothing, got %d", len(got))
	}
}

func TestScrollDown(t *testing.T) {
	app := newTestApp(t, goodResult(time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)), nil, nil, nil)
	if app.offset != 0 {
		t.Fatalf("initial offset %d", app.offset)
	}
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if app.offset != 1 {
		t.Fatalf("j must scroll down, offset=%d", app.offset)
	}
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if app.offset != 0 {
		t.Fatalf("k must scroll up, offset=%d", app.offset)
	}
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if app.offset != 0 {
		t.Fatalf("offset must clamp at 0, offset=%d", app.offset)
	}
}

func TestBigDigitsRender(t *testing.T) {
	rows := bigDigits("21.50")
	if len(rows) != 3 {
		t.Fatalf("hero digits must be 3 rows tall, got %d", len(rows))
	}
	widths := map[int]bool{}
	for _, r := range rows {
		if strings.TrimSpace(r) == "" {
			t.Errorf("empty hero row: %q", rows)
		}
		widths[len([]rune(r))] = true
	}
	if len(widths) != 1 {
		t.Errorf("hero rows must be equal width, got %v", widths)
	}
	if !strings.Contains(strings.Join(rows, ""), "█") {
		t.Errorf("hero digits must use block glyphs, got %q", rows)
	}
}

func TestHeroIsTallerThanOneLine(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	app := newTestApp(t, goodResult(now), nil, nil, nil)
	hero := heroRow(app, aggFor(app), 88)
	if n := len(strings.Split(hero, "\n")); n < 3 {
		t.Fatalf("hero must occupy >= 3 rows, got %d:\n%s", n, hero)
	}
}

func TestCompactToggleHidesMetricsAndTabs(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	app := newTestApp(t, goodResult(now), nil, nil, nil)
	app.compact = false

	full := app.View()
	if !strings.Contains(full, "BURN RATE") || !strings.Contains(full, "Overview") {
		t.Fatal("full mode must show metrics and tabs")
	}

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	compact := app.View()
	if strings.Contains(compact, "BURN RATE") {
		t.Error("compact mode must hide the metrics row")
	}
	if strings.Contains(compact, "Overview") {
		t.Error("compact mode must hide the tab bar")
	}
	if !strings.Contains(compact, "spent") {
		t.Error("compact mode must keep the budget summary")
	}

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if !strings.Contains(app.View(), "BURN RATE") {
		t.Error("c must toggle back to full mode")
	}
}

func TestHeroBlockSuffixSymbol(t *testing.T) {
	rows := heroBlock("12.90", "$")
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	mid := rows[len(rows)/2]
	if !strings.HasSuffix(strings.TrimRight(mid, " "), "$") {
		t.Errorf("symbol must be a suffix on the middle row, got %q", mid)
	}
	if strings.HasPrefix(strings.TrimSpace(rows[0]), "$") {
		t.Errorf("symbol must not be a prefix, got %q", rows[0])
	}
	for _, r := range rows {
		if len([]rune(r)) != len([]rune(rows[0])) {
			t.Errorf("rows must be equal width: %q", rows)
		}
	}
	// Percentages keep working the same way.
	pct := heroBlock("21.5", "%")
	if !strings.HasSuffix(strings.TrimRight(pct[len(pct)/2], " "), "%") {
		t.Errorf("percent suffix broken: %q", pct[1])
	}
}

func TestCompactHidesOverviewBody(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	app := newTestApp(t, goodResult(now), nil, nil, nil)

	app.compact = false
	full := app.View()
	for _, want := range []string{"DAILY SPEND", "TOP MODELS"} {
		if !strings.Contains(full, want) {
			t.Fatalf("full mode must show %q", want)
		}
	}

	app.compact = true
	compact := app.View()
	for _, gone := range []string{"DAILY SPEND", "TOP MODELS"} {
		if strings.Contains(compact, gone) {
			t.Errorf("compact mode must hide %q", gone)
		}
	}
	// The glanceable summary survives.
	for _, keep := range []string{"of $", "spent ", "left ", "pace"} {
		if !strings.Contains(compact, keep) {
			t.Errorf("compact mode must keep %q", keep)
		}
	}
}

func TestCompactIsDefault(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	app := newTestApp(t, goodResult(now), nil, nil, nil)
	if !app.compact {
		t.Fatal("compact mode must be on by default")
	}
	view := app.View()
	if strings.Contains(view, "BURN RATE") || strings.Contains(view, "DAILY SPEND") {
		t.Error("default view must start compact")
	}
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if !strings.Contains(app.View(), "BURN RATE") {
		t.Error("c must expand to full mode")
	}
}

func TestFooterNamesCompactToggle(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	app := newTestApp(t, goodResult(now), nil, nil, nil)
	if !strings.Contains(app.View(), "compact toggle") {
		t.Error("footer must label the key as a compact toggle")
	}
}

func TestHeroRightAlignsPercentBlock(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	app := newTestApp(t, goodResult(now), nil, nil, nil)
	hero := stripStyles(heroRow(app, aggFor(app), 88))
	lines := strings.Split(hero, "\n")
	if len(lines) < 3 {
		t.Fatalf("hero too short: %q", hero)
	}
	want := len([]rune(lines[0]))
	for i, l := range lines {
		if got := len([]rune(l)); got != want {
			t.Errorf("hero line %d width %d != %d (percent block must share the right edge):\n%s", i, got, want, hero)
		}
	}
	// The block must also reach the panel's right edge, not stop short.
	const inner = 88
	if right := len([]rune(strings.TrimRight(lines[0], " "))); right > inner || right < inner-1 {
		t.Errorf("hero right edge at %d, want ~%d:\n%s", right, inner, hero)
	}
}

func stripStyles(s string) string {
	re := regexp.MustCompile("\x1b\\[[0-9;?]*[a-zA-Z]")
	return re.ReplaceAllString(s, "")
}

func TestSymmetricHorizontalPadding(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	res := goodResult(now)
	res.Period.PeriodStartMs = time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	res.Period.PeriodEndMs = time.Date(2026, 12, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	app := newTestApp(t, res, nil, nil, nil)
	app.width = 100

	const (
		pad   = 2
		width = 100
	)
	var sawRule bool
	for _, line := range strings.Split(app.View(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue // blank spacer rows carry no content
		}
		runes := []rune(line)
		if len(runes) != width {
			t.Fatalf("line must fill the canvas: %d %q", len(runes), line)
		}
		lead := len(runes) - len([]rune(strings.TrimLeft(line, " ")))
		trail := len(runes) - len([]rune(strings.TrimRight(line, " ")))
		// Glyph side bearings may indent a numeral, never less than the gutter.
		if lead < pad {
			t.Errorf("left gutter %d < %d: %q", lead, pad, line)
		}
		if trail < pad {
			t.Errorf("right gutter %d < %d: %q", trail, pad, line)
		}
		// Structural rules must span exactly [pad, width-pad).
		if strings.Contains(line, "─") {
			sawRule = true
			if lead != pad || trail != pad {
				t.Errorf("rule padding l=%d r=%d want %d/%d: %q", lead, trail, pad, pad, line)
			}
		}
	}
	if !sawRule {
		t.Error("no structural rule found to measure padding against")
	}
}
