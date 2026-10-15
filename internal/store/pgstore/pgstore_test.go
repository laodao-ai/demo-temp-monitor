package pgstore

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/laodao-ai/demo-temp-monitor/internal/store"
)

func TestMapErr(t *testing.T) {
	plain := errors.New("connection reset")
	otherCode := &pgconn.PgError{Code: "23514"}
	cases := []struct {
		name     string
		err      error
		code     string
		sentinel error
		want     error
	}{
		{"unique violation", &pgconn.PgError{Code: "23505"}, "23505", store.ErrAlreadyExists, store.ErrAlreadyExists},
		{"foreign key violation", &pgconn.PgError{Code: "23503"}, "23503", store.ErrNotFound, store.ErrNotFound},
		{"wrapped unique violation", fmt.Errorf("exec: %w", &pgconn.PgError{Code: "23505"}), "23505", store.ErrAlreadyExists, store.ErrAlreadyExists},
		{"other code is not mapped", otherCode, "23505", store.ErrAlreadyExists, otherCode},
		{"non-pg error is not mapped", plain, "23505", store.ErrAlreadyExists, plain},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := mapErr(c.err, c.code, c.sentinel); !errors.Is(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}
