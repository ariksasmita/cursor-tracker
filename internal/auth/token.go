// Package auth resolves the Cursor API token from env, macOS keychain,
// or the Cursor app's state.vscdb.
package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// KeychainCmd runs the macOS security CLI to fetch the cursor access token.
// Package var so tests can stub it.
var KeychainCmd = func(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "security", "find-generic-password",
		"-a", "cursor-user", "-s", "cursor-access-token", "-w")
	return cmd.Output()
}

// StateDbPath is the Cursor app state DB read as a fallback.
var StateDbPath = defaultStateDbPath()

func defaultStateDbPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "Cursor",
		"User", "globalStorage", "state.vscdb")
}

// ResolveToken returns a Cursor API token and its origin, trying in order:
// CURSOR_TOKEN env, macOS keychain, state.vscdb ItemTable.
func ResolveToken(ctx context.Context) (string, string, error) {
	var errs []string

	if tok := strings.TrimSpace(os.Getenv("CURSOR_TOKEN")); tok != "" {
		return tok, "env", nil
	}
	errs = append(errs, "env: CURSOR_TOKEN not set")

	if out, err := KeychainCmd(ctx); err == nil {
		if tok := strings.TrimSpace(string(out)); tok != "" {
			return tok, "keychain", nil
		}
		errs = append(errs, "keychain: empty token")
	} else {
		errs = append(errs, "keychain: "+err.Error())
	}

	if tok, err := tokenFromStateDB(); err != nil {
		errs = append(errs, "state.vscdb: "+err.Error())
	} else if tok != "" {
		return tok, "state.vscdb", nil
	} else {
		errs = append(errs, "state.vscdb: empty token")
	}

	return "", "", fmt.Errorf("no cursor token available: %s", strings.Join(errs, "; "))
}

func tokenFromStateDB() (string, error) {
	if StateDbPath == "" {
		return "", errors.New("state db path unknown")
	}
	db, err := sql.Open("sqlite", "file:"+StateDbPath+"?mode=ro")
	if err != nil {
		return "", err
	}
	defer db.Close()
	var tok string
	err = db.QueryRow(
		"SELECT value FROM ItemTable WHERE key='cursorAuth/accessToken'").Scan(&tok)
	if err != nil {
		return "", err
	}
	return tok, nil
}
