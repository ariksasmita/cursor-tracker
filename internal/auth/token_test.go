package auth

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

var errStub = errors.New("stub keychain failure")

func seedStateDB(t *testing.T, path, token string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS ItemTable (key TEXT PRIMARY KEY, value TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO ItemTable (key, value) VALUES ('cursorAuth/accessToken', ?)", token); err != nil {
		t.Fatal(err)
	}
}

func makeStateDB(t *testing.T, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.vscdb")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	seedStateDB(t, path, token)
	return path
}

func stubSources(t *testing.T, keychainOut []byte, stateDb string) func() {
	t.Helper()
	oldCmd, oldPath := KeychainCmd, StateDbPath
	if keychainOut == nil {
		KeychainCmd = func(ctx context.Context) ([]byte, error) { return nil, errStub }
	} else {
		KeychainCmd = func(ctx context.Context) ([]byte, error) { return keychainOut, nil }
	}
	StateDbPath = stateDb
	return func() { KeychainCmd, StateDbPath = oldCmd, oldPath }
}

func TestResolveToken_Env(t *testing.T) {
	t.Setenv("CURSOR_TOKEN", "tok-env")
	reset := stubSources(t, nil, makeStateDB(t, "tok-db"))
	defer reset()
	tok, origin, err := ResolveToken(context.Background())
	if err != nil || tok != "tok-env" || origin != "env" {
		t.Fatalf("got %q %q %v", tok, origin, err)
	}
}

func TestResolveToken_Keychain(t *testing.T) {
	t.Setenv("CURSOR_TOKEN", "")
	reset := stubSources(t, []byte("tok-key\n"), makeStateDB(t, "tok-db"))
	defer reset()
	tok, origin, err := ResolveToken(context.Background())
	if err != nil || tok != "tok-key" || origin != "keychain" {
		t.Fatalf("got %q %q %v", tok, origin, err)
	}
}

func TestResolveToken_StateDB(t *testing.T) {
	t.Setenv("CURSOR_TOKEN", "")
	reset := stubSources(t, nil, makeStateDB(t, "tok-db"))
	defer reset()
	tok, origin, err := ResolveToken(context.Background())
	if err != nil || tok != "tok-db" || origin != "state.vscdb" {
		t.Fatalf("got %q %q %v", tok, origin, err)
	}
}

func TestResolveToken_AllFail(t *testing.T) {
	t.Setenv("CURSOR_TOKEN", "")
	reset := stubSources(t, nil, filepath.Join(t.TempDir(), "missing.vscdb"))
	defer reset()
	_, _, err := ResolveToken(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"env", "keychain", "state.vscdb"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}
