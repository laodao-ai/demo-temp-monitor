// Package testdb is the shared setup for tests that talk to the real database.
// Every failure is t.Fatal with the next step to take. It never skips: a
// missing database must show up as a failing test (TST-06).
package testdb

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/laodao-ai/demo-temp-monitor/internal/config"
	"github.com/laodao-ai/demo-temp-monitor/internal/redact"
	"github.com/laodao-ai/demo-temp-monitor/internal/store/pgstore"
)

const (
	envFileName = "config/local.env"

	hintNoURL    = "copy config/example.env to config/local.env and fill it in"
	hintConnect  = "check the SSH tunnel and that the URL uses the session pool port 7432"
	hintNoTables = "run scripts/db-reset.sh first"
)

// Open finds the repo root, reads the configuration, connects within 5 seconds,
// checks that the tables exist and returns a pool (MaxConns=4) that is closed
// when the test ends.
func Open(t *testing.T) *pgxpool.Pool {
	t.Helper()

	root, err := findRoot()
	if err != nil {
		t.Fatalf("cannot locate the repo root: %v", err)
	}
	envPath := filepath.Join(root, filepath.FromSlash(envFileName))
	cfg, err := config.Load(os.Getenv, envPath)
	if err != nil {
		t.Fatal(configFailure(err, envPath))
	}

	pool, err := pgstore.Connect(t.Context(), cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("%s; %s", err, hintConnect)
	}
	t.Cleanup(pool.Close)

	if err := pgstore.CheckSchema(t.Context(), pool); err != nil {
		if errors.Is(err, pgstore.ErrSchemaMissing) {
			t.Fatalf("%s; %s", err, hintNoTables)
		}
		text, ok := redact.Text(err.Error(), cfg.DatabaseURL)
		if !ok {
			text = "error details withheld"
		}
		t.Fatalf("cannot check database schema: %s", text)
	}
	return pool
}

// findRoot walks up from the working directory (the package directory under
// go test) to the directory that holds go.mod.
func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod found in any parent directory")
		}
		dir = parent
	}
}

// configFailure turns a config.Load error into "<text>; <next step>". The
// absolute path is shown as config/local.env, as design "Error Messages" says.
func configFailure(err error, envPath string) string {
	msg := strings.ReplaceAll(err.Error(), envPath, envFileName)
	switch {
	case strings.Contains(msg, config.KeyDatabaseURL+" is not set"):
		return msg + "; " + hintNoURL
	case strings.HasSuffix(msg, `missing "="`):
		return msg + "; use KEY=VALUE"
	case strings.HasSuffix(msg, "unknown key"):
		return msg + "; allowed keys: " + strings.Join([]string{config.KeyDatabaseURL, config.KeyListenAddr, config.KeyThreshold}, ", ")
	case strings.HasSuffix(msg, "must not be quoted"):
		return msg + "; remove the quotes"
	default:
		return msg
	}
}
