// Package pgstore is the PostgreSQL implementation of store.Store.
// It never checks before writing: the database constraints decide, and
// SQLSTATE 23505 / 23503 are mapped to the store sentinel errors.
package pgstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/laodao-ai/demo-temp-monitor/internal/store"
)

const (
	// ConnectTimeout bounds connecting, the first ping, and the schema check.
	ConnectTimeout = 5 * time.Second
	// maxConns keeps every client pool small: the PgBouncer session pool gives
	// each client connection a dedicated backend and has a fixed total limit.
	maxConns = 4

	sqlStateUniqueViolation     = "23505"
	sqlStateForeignKeyViolation = "23503"
)

var (
	// ErrConnect means the pool could not be built or the database did not
	// answer a ping. It carries no driver text on purpose: the driver can echo
	// parts of a malformed password as a host name.
	ErrConnect = errors.New("cannot connect to database")
	// ErrSchemaMissing means devices or readings does not exist.
	ErrSchemaMissing = errors.New("table devices or readings not found")
)

// Connect builds a pool with MaxConns=4 and pings it within ConnectTimeout.
func Connect(ctx context.Context, connURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(connURL)
	if err != nil {
		return nil, ErrConnect
	}
	cfg.MaxConns = maxConns

	ctx, cancel := context.WithTimeout(ctx, ConnectTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, ErrConnect
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, ErrConnect
	}
	return pool, nil
}

// CheckSchema returns ErrSchemaMissing unless both tables exist.
// Any other error comes from the driver and must be redacted before output.
func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(ctx, ConnectTimeout)
	defer cancel()
	var ok bool
	err := pool.QueryRow(ctx,
		`SELECT to_regclass('devices') IS NOT NULL AND to_regclass('readings') IS NOT NULL`,
	).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrSchemaMissing
	}
	return nil
}

// Store implements store.Store on a pgx pool.
type Store struct {
	pool *pgxpool.Pool
}

// New returns a Store that uses pool. The caller owns and closes the pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) CreateDevice(ctx context.Context, id string) (store.Device, error) {
	var d store.Device
	err := s.pool.QueryRow(ctx,
		`INSERT INTO devices (id) VALUES ($1) RETURNING id, registered_at`, id,
	).Scan(&d.ID, &d.RegisteredAt)
	if err != nil {
		return store.Device{}, mapErr("create device", err, sqlStateUniqueViolation, store.ErrAlreadyExists)
	}
	return d, nil
}

func (s *Store) DeleteDevice(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM devices WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete device: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) AddReading(ctx context.Context, deviceID string, tempC float64) (store.Reading, error) {
	var r store.Reading
	err := s.pool.QueryRow(ctx,
		`INSERT INTO readings (device_id, temperature_c) VALUES ($1, $2)
		 RETURNING id, device_id, temperature_c, recorded_at`, deviceID, tempC,
	).Scan(&r.ID, &r.DeviceID, &r.TemperatureC, &r.RecordedAt)
	if err != nil {
		return store.Reading{}, mapErr("add reading", err, sqlStateForeignKeyViolation, store.ErrNotFound)
	}
	return r, nil
}

func (s *Store) ListReadings(ctx context.Context, deviceID string) ([]store.Reading, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, device_id, temperature_c, recorded_at
		 FROM readings WHERE device_id = $1 ORDER BY id`, deviceID)
	if err != nil {
		return nil, fmt.Errorf("list readings: %w", err)
	}
	defer rows.Close()
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (store.Reading, error) {
		var r store.Reading
		err := row.Scan(&r.ID, &r.DeviceID, &r.TemperatureC, &r.RecordedAt)
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("list readings: %w", err)
	}
	return out, nil
}

// mapErr returns sentinel when err is a PostgreSQL error with the given
// SQLSTATE. Any other error is wrapped with op, so the log shows which
// statement failed; %w keeps errors.Is/As and error-value redaction working.
func mapErr(op string, err error, sqlState string, sentinel error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == sqlState {
		return sentinel
	}
	return fmt.Errorf("%s: %w", op, err)
}
