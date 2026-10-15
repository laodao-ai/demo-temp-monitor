//go:build integration

package pgstore_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/laodao-ai/demo-temp-monitor/internal/store"
	"github.com/laodao-ai/demo-temp-monitor/internal/store/pgstore"
	"github.com/laodao-ai/demo-temp-monitor/internal/store/storetest"
	"github.com/laodao-ai/demo-temp-monitor/internal/testdb"
)

// TestIntegrationContract runs the shared store contract on PostgreSQL.
// The suite makes its own it-<random> device IDs and deletes them.
func TestIntegrationContract(t *testing.T) {
	pool := testdb.Open(t)
	storetest.Run(t, func(t *testing.T) store.Store { return pgstore.New(pool) })
}

// TestIntegrationConstraints covers DB-05: the database enforces the rules
// itself, with plain SQL and without the service code.
func TestIntegrationConstraints(t *testing.T) {
	pool := testdb.Open(t)

	t.Run("CheckRejectsBadID", func(t *testing.T) {
		// Cleanup first: if the constraint is missing, the row must not stay.
		cleanupDevice(t, pool, "Bad_ID")
		_, err := pool.Exec(t.Context(), `INSERT INTO devices (id) VALUES ($1)`, "Bad_ID")
		requireSQLState(t, err, "23514") // check_violation
	})

	t.Run("PrimaryKeyRejectsDuplicate", func(t *testing.T) {
		id := newTestID(t)
		cleanupDevice(t, pool, id)
		if _, err := pool.Exec(t.Context(), `INSERT INTO devices (id) VALUES ($1)`, id); err != nil {
			t.Fatal(err)
		}
		_, err := pool.Exec(t.Context(), `INSERT INTO devices (id) VALUES ($1)`, id)
		requireSQLState(t, err, "23505") // unique_violation
	})

	t.Run("ForeignKeyRejectsOrphanReading", func(t *testing.T) {
		id := newTestID(t) // never inserted
		_, err := pool.Exec(t.Context(),
			`INSERT INTO readings (device_id, temperature_c) VALUES ($1, 20.0)`, id)
		requireSQLState(t, err, "23503") // foreign_key_violation
	})

	t.Run("DeleteCascadesReadings", func(t *testing.T) {
		id := newTestID(t)
		cleanupDevice(t, pool, id)
		ctx := t.Context()
		if _, err := pool.Exec(ctx, `INSERT INTO devices (id) VALUES ($1)`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO readings (device_id, temperature_c) VALUES ($1, 20.0), ($1, 60.0)`, id); err != nil {
			t.Fatal(err)
		}
		if got := countReadings(t, pool, id); got != 2 {
			t.Fatalf("readings before delete = %d, want 2", got)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM devices WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		if got := countReadings(t, pool, id); got != 0 {
			t.Fatalf("readings after delete = %d, want 0", got)
		}
	})
}

func newTestID(t *testing.T) string {
	t.Helper()
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return "it-" + hex.EncodeToString(b[:])
}

// cleanupDevice deletes the device when the test ends. It uses its own
// context: t.Context() is already canceled while cleanups run.
func cleanupDevice(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, `DELETE FROM devices WHERE id = $1`, id); err != nil {
			t.Errorf("cleanup of %s: %v", id, err)
		}
	})
}

func countReadings(t *testing.T, pool *pgxpool.Pool, deviceID string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM readings WHERE device_id = $1`, deviceID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func requireSQLState(t *testing.T, err error, want string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("error = %v, want a PostgreSQL error with SQLSTATE %s", err, want)
	}
	if pgErr.Code != want {
		t.Fatalf("SQLSTATE = %s, want %s", pgErr.Code, want)
	}
}
