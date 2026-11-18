package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sasmitai/cursor-tracker/internal/auth"
)

func TestBuildDeps_HappyPath(t *testing.T) {
	t.Setenv("CURSOR_TOKEN", "tok-env")
	t.Setenv("CURSOR_TRACKER_HOME", t.TempDir())
	deps, warnings, err := BuildDeps(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v (warnings %v)", err, warnings)
	}
	if deps.Primary == nil || deps.Primary.Name() != "live" {
		t.Fatalf("want live primary, got %v", deps.Primary)
	}
	if deps.Store == nil {
		t.Error("history store must be opened")
	}
	for _, w := range warnings {
		if strings.Contains(w, "token") {
			t.Errorf("unexpected token warning: %v", w)
		}
	}
}

func TestBuildDeps_DegradesWithoutToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CURSOR_TOKEN", "")
	// Stub all token sources to fail.
	oldCmd, oldPath := auth.KeychainCmd, auth.StateDbPath
	auth.KeychainCmd = func(ctx context.Context) ([]byte, error) { return nil, os.ErrPermission }
	auth.StateDbPath = filepath.Join(home, "missing.vscdb")
	t.Cleanup(func() { auth.KeychainCmd, auth.StateDbPath = oldCmd, oldPath })

	t.Setenv("CURSOR_TRACKER_HOME", home)
	deps, warnings, err := BuildDeps(context.Background())
	if err != nil {
		t.Fatalf("must degrade instead of failing: %v", err)
	}
	if deps.Primary != nil {
		t.Errorf("primary should be nil without token, got %v", deps.Primary)
	}
	joined := strings.Join(warnings, " | ")
	if !strings.Contains(joined, "token") {
		t.Errorf("warnings must mention token failure: %v", warnings)
	}
}

func TestBuildDeps_MissingOpenBotIsWarningNotError(t *testing.T) {
	t.Setenv("CURSOR_TOKEN", "tok-env")
	t.Setenv("CURSOR_TRACKER_HOME", t.TempDir())
	t.Setenv("OPENBOT_DB", filepath.Join(t.TempDir(), "absent.db"))
	deps, warnings, err := BuildDeps(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deps.Sessions != nil {
		t.Error("sessions source should be nil when openbot.db is absent")
	}
	if !strings.Contains(strings.Join(warnings, " | "), "openbot.db not found") {
		t.Errorf("want openbot warning, got %v", warnings)
	}
}
