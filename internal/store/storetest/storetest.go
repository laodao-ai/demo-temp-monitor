// Package storetest is the contract test suite run against every store.Store
// implementation.
package storetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/laodao-ai/demo-temp-monitor/internal/store"
)

const (
	concurrency = 20
	// raceRounds repeats the delete-vs-report case; one round rarely hits the race window.
	raceRounds = 25
)

// Run executes the contract against stores made by newStore. The returned
// store may already hold other data, so each case uses its own random device ID.
func Run(t *testing.T, newStore func(t *testing.T) store.Store) {
	cases := []struct {
		name string
		fn   func(t *testing.T, s store.Store)
	}{
		{"CreateDevice", testCreate},
		{"CreateDuplicate", testCreateDuplicate},
		{"DeleteCascadesReadings", testDeleteCascades},
		{"DeleteMissing", testDeleteMissing},
		{"AddReadingMissingDevice", testAddReadingMissing},
		{"ReadingsInWriteOrder", testReadingOrder},
		{"ConcurrentCreateSameID", testConcurrentCreate},
		{"ConcurrentAddReading", testConcurrentAddReading},
		{"ConcurrentDeleteAndReport", testConcurrentDeleteAndReport},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.fn(t, newStore(t))
		})
	}
}

// NewID returns a random "it-<hex>" device ID that matches the ID pattern.
// It registers no cleanup; callers that insert the device must delete it.
func NewID(t *testing.T) string {
	t.Helper()
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return "it-" + hex.EncodeToString(b[:])
}

// newID returns NewID() and deletes that device when the case ends.
func newID(t *testing.T, s store.Store) string {
	t.Helper()
	id := NewID(t)
	t.Cleanup(func() {
		// Independent context: t.Context() is already canceled during cleanup.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.DeleteDevice(ctx, id); err != nil && !errors.Is(err, store.ErrNotFound) {
			t.Errorf("cleanup of %s: %v", id, err)
		}
	})
	return id
}

func testCreate(t *testing.T, s store.Store) {
	ctx := t.Context()
	id := newID(t, s)
	d, err := s.CreateDevice(ctx, id)
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	if d.ID != id || d.RegisteredAt.IsZero() {
		t.Fatalf("device = %+v", d)
	}
}

func testCreateDuplicate(t *testing.T, s store.Store) {
	ctx := t.Context()
	id := newID(t, s)
	if _, err := s.CreateDevice(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDevice(ctx, id); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("second CreateDevice err = %v, want ErrAlreadyExists", err)
	}
}

func testDeleteCascades(t *testing.T, s store.Store) {
	ctx := t.Context()
	id := newID(t, s)
	if _, err := s.CreateDevice(ctx, id); err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{20, 30} {
		if _, err := s.AddReading(ctx, id, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteDevice(ctx, id); err != nil {
		t.Fatalf("DeleteDevice: %v", err)
	}
	got, err := s.ListReadings(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("readings left after delete: %d", len(got))
	}
	// Re-registering must give an empty history, not the old readings.
	if _, err := s.CreateDevice(ctx, id); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	got, err = s.ListReadings(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("re-registered device has %d old readings", len(got))
	}
}

func testDeleteMissing(t *testing.T, s store.Store) {
	id := newID(t, s)
	if err := s.DeleteDevice(t.Context(), id); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeleteDevice err = %v, want ErrNotFound", err)
	}
}

func testAddReadingMissing(t *testing.T, s store.Store) {
	id := newID(t, s)
	if _, err := s.AddReading(t.Context(), id, 21.5); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("AddReading err = %v, want ErrNotFound", err)
	}
}

func testReadingOrder(t *testing.T, s store.Store) {
	ctx := t.Context()
	id := newID(t, s)
	if _, err := s.CreateDevice(ctx, id); err != nil {
		t.Fatal(err)
	}
	want := []float64{10.5, -3, 99.25, 0, 50}
	for _, v := range want {
		r, err := s.AddReading(ctx, id, v)
		if err != nil {
			t.Fatal(err)
		}
		if r.DeviceID != id || r.TemperatureC != v || r.RecordedAt.IsZero() {
			t.Fatalf("AddReading returned %+v for %v", r, v)
		}
	}
	got, err := s.ListReadings(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d readings, want %d", len(got), len(want))
	}
	for i, r := range got {
		if r.TemperatureC != want[i] {
			t.Errorf("reading %d = %v, want %v", i, r.TemperatureC, want[i])
		}
		if i > 0 && r.ID <= got[i-1].ID {
			t.Errorf("reading IDs not ascending: %d after %d", r.ID, got[i-1].ID)
		}
	}
}

// testListReadingsCopy checks that writing to a returned slice does not change
// what the next ListReadings returns.
func testListReadingsCopy(t *testing.T, s store.Store) {
	ctx := t.Context()
	id := newID(t, s)
	if _, err := s.CreateDevice(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddReading(ctx, id, 21.5); err != nil {
		t.Fatal(err)
	}
	first, err := s.ListReadings(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("got %d readings, want 1", len(first))
	}
	first[0].TemperatureC = -999
	second, err := s.ListReadings(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].TemperatureC != 21.5 {
		t.Fatalf("second ListReadings = %+v, want one reading of 21.5", second)
	}
}

// fanOut runs fn(i) for i in [0, n) in parallel and returns the errors by index.
func fanOut(n int, fn func(i int) error) []error {
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = fn(i)
		}()
	}
	wg.Wait()
	return errs
}

func testConcurrentCreate(t *testing.T, s store.Store) {
	ctx := t.Context()
	id := newID(t, s)
	errs := fanOut(concurrency, func(int) error {
		_, err := s.CreateDevice(ctx, id)
		return err
	})
	ok, dup := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, store.ErrAlreadyExists):
			dup++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 || dup != concurrency-1 {
		t.Fatalf("succeeded %d, already-exists %d; want 1 and %d", ok, dup, concurrency-1)
	}
}

func testConcurrentAddReading(t *testing.T, s store.Store) {
	ctx := t.Context()
	id := newID(t, s)
	if _, err := s.CreateDevice(ctx, id); err != nil {
		t.Fatal(err)
	}
	errs := fanOut(concurrency, func(i int) error {
		_, err := s.AddReading(ctx, id, float64(i))
		return err
	})
	for _, err := range errs {
		if err != nil {
			t.Errorf("AddReading: %v", err)
		}
	}
	got, err := s.ListReadings(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != concurrency {
		t.Fatalf("got %d readings, want %d", len(got), concurrency)
	}
}

func testConcurrentDeleteAndReport(t *testing.T, s store.Store) {
	ctx := t.Context()
	for round := 0; round < raceRounds; round++ {
		id := newID(t, s)
		if _, err := s.CreateDevice(ctx, id); err != nil {
			t.Fatal(err)
		}
		// Index `concurrency` is the delete; the rest are reports.
		results := fanOut(concurrency+1, func(i int) error {
			if i == concurrency {
				return s.DeleteDevice(ctx, id)
			}
			_, err := s.AddReading(ctx, id, float64(i))
			return err
		})
		errs, delErr := results[:concurrency], results[concurrency]

		if delErr != nil {
			t.Fatalf("round %d: DeleteDevice: %v", round, delErr)
		}
		for _, err := range errs {
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("round %d: AddReading err = %v, want nil or ErrNotFound", round, err)
			}
		}
		got, err := s.ListReadings(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("round %d: %d readings left after delete", round, len(got))
		}
	}
}
