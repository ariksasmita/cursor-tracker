// Package app wires data sources, history, and the TUI together.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ariksasmita/cursor-tracker/internal/auth"
	"github.com/ariksasmita/cursor-tracker/internal/history"
	"github.com/ariksasmita/cursor-tracker/internal/source"
	"github.com/ariksasmita/cursor-tracker/internal/tui"
)

const (
	defaultEndpoint = "https://api2.cursor.sh"
	defaultOpenBot  = "openbot.db"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// trackerHome is the directory holding the tracker's own data.
func trackerHome() string {
	return envOr("CURSOR_TRACKER_HOME", filepath.Join(homeDir(), ".cursor-tracker"))
}

// stateDBPath is the Cursor app state DB used for tokens and snapshots.
func stateDBPath() string {
	return envOr("CURSOR_STATE_DB", auth.StateDbPath)
}

// openBotPath is OpenBot's database path.
func openBotPath() string {
	if v := os.Getenv("OPENBOT_DB"); v != "" {
		return v
	}
	return filepath.Join(homeDir(), "Library", "Application Support", "OpenBot", defaultOpenBot)
}

// BuildDeps assembles TUI dependencies. Missing optional sources never fail
// the build: they degrade into warnings.
func BuildDeps(ctx context.Context) (tui.Deps, []string, error) {
	var warnings []string
	var deps tui.Deps

	if store, err := history.Open(filepath.Join(trackerHome(), "history.db")); err != nil {
		warnings = append(warnings, "history unavailable: "+err.Error())
	} else {
		deps.Store = store
	}

	statePath := stateDBPath()
	if snap, err := source.NewLocalSnapshot(statePath); err != nil {
		warnings = append(warnings, "snapshot unavailable: "+err.Error())
	} else {
		deps.Fallback = snap
	}

	if token, _, terr := auth.ResolveToken(ctx); terr != nil {
		warnings = append(warnings, "no token: "+terr.Error())
	} else {
		deps.Primary = source.NewLiveAPI(envOr("CURSOR_API_ENDPOINT", defaultEndpoint), token)
	}

	sessPath := openBotPath()
	if _, err := os.Stat(sessPath); err != nil {
		warnings = append(warnings, "openbot.db not found at "+sessPath)
	} else if ob, err := source.NewOpenBot(sessPath); err != nil {
		warnings = append(warnings, "openbot unavailable: "+err.Error())
	} else {
		deps.Sessions = ob
	}

	if deps.Primary == nil && deps.Fallback == nil {
		warnings = append(warnings, "no usage source available")
	}
	return deps, warnings, nil
}

// Run starts the TUI and blocks until the user quits.
func Run(ctx context.Context) error {
	deps, warnings, err := BuildDeps(ctx)
	if err != nil {
		return err
	}
	app := tui.New(deps)
	app.SetWarnings(warnings)
	// Hide the hardware cursor: bubbletea parks it mid-frame, where it
	// renders as a stray block inside blank glyph cells.
	fmt.Print("\x1b[?25l")
	defer fmt.Print("\x1b[?25h")
	_, err = tea.NewProgram(app, tea.WithAltScreen()).Run()
	return err
}
