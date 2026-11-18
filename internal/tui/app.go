// Package tui renders the cursor-tracker dashboard as a Bubble Tea app.
package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sasmitai/cursor-tracker/internal/history"
	"github.com/sasmitai/cursor-tracker/internal/source"
)

// Deps wires the app to its data sources.
type Deps struct {
	Primary  UsageSourceOptional
	Fallback UsageSourceOptional
	Sessions SessionSourceOptional
	Store    *history.Store
	Now      func() time.Time
}

// UsageSourceOptional is a UsageSource that may be absent.
type UsageSourceOptional interface {
	source.UsageSource
}

// SessionSourceOptional is a SessionSource that may be absent.
type SessionSourceOptional interface {
	source.SessionSource
}

// FetchResult is one completed data pull.
type FetchResult struct {
	Period       *source.PeriodUsage
	Daily        []source.DailySpend
	Sessions     []source.SessionEvent
	SessionsErr  error
	SourceName   string
	FetchedAt    time.Time
	PrimaryError error // non-nil when primary source failed and fallback used
}

// App is the root tea.Model.
type App struct {
	deps    Deps
	tab     int
	result  FetchResult
	warning string

	// Fetch pulls data; a field so tests inject results without IO.
	Fetch func(ctx context.Context) (FetchResult, error)
}

// New builds the App.
func New(deps Deps) *App {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	a := &App{deps: deps}
	a.Fetch = defaultFetch(deps)
	return a
}

type tickMsg time.Time

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	return tea.Batch(a.doFetch(), tea.Tick(time.Minute, func(t time.Time) tea.Msg { return tickMsg(t) }))
}

func (a *App) doFetch() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		res, err := a.Fetch(ctx)
		if err != nil {
			return fetchErrMsg{err}
		}
		return res
	}
}

type fetchErrMsg struct{ err error }

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		switch m.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		case "r":
			return a, a.doFetch()
		case "1", "2", "3", "4":
			a.switchTab(int(m.String()[0] - '0'))
		}
	case tickMsg:
		return a, tea.Tick(time.Minute, func(t time.Time) tea.Msg { return tickMsg(t) })
	case FetchResult:
		a.loadResult(m)
	case fetchErrMsg:
		a.warning = "fetch failed: " + m.err.Error()
	}
	return a, nil
}

// loadResult stores a fetch outcome and persists it to history.
func (a *App) loadResult(res FetchResult) {
	a.result = res
	a.warning = ""
	if res.PrimaryError != nil {
		a.warning = "live unavailable (" + res.PrimaryError.Error() + "), showing " + res.SourceName
	}
	if a.deps.Store != nil && res.Period != nil {
		_ = a.deps.Store.UpsertPeriod(*res.Period)
		if len(res.Daily) > 0 {
			_ = a.deps.Store.UpsertDaily(res.Period.PeriodStartMs, res.Daily)
		}
	}
}

// SetWarnings records startup warnings shown above the dashboard.
func (a *App) SetWarnings(warnings []string) {
	a.warning = strings.Join(warnings, "\u00b7 ")
}

func (a *App) switchTab(n int) {
	if n >= 1 && n <= 4 {
		a.tab = n
	}
}

// View implements tea.Model.
func (a *App) View() string {
	return render(a)
}
