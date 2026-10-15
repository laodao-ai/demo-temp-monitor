// Package memstore is the in-memory store.Store. One mutex guards everything.
package memstore

import (
	"context"
	"sync"
	"time"

	"github.com/laodao-ai/demo-temp-monitor/internal/store"
)

type Store struct {
	mu       sync.Mutex
	devices  map[string]store.Device
	readings map[string][]store.Reading
	nextID   int64
}

func New() *Store {
	return &Store{
		devices:  map[string]store.Device{},
		readings: map[string][]store.Reading{},
	}
}

func (s *Store) CreateDevice(_ context.Context, id string) (store.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[id]; ok {
		return store.Device{}, store.ErrAlreadyExists
	}
	d := store.Device{ID: id, RegisteredAt: time.Now().UTC()}
	s.devices[id] = d
	return d, nil
}

func (s *Store) DeleteDevice(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.devices, id)
	delete(s.readings, id)
	return nil
}

func (s *Store) AddReading(_ context.Context, deviceID string, tempC float64) (store.Reading, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[deviceID]; !ok {
		return store.Reading{}, store.ErrNotFound
	}
	s.nextID++
	r := store.Reading{ID: s.nextID, DeviceID: deviceID, TemperatureC: tempC, RecordedAt: time.Now().UTC()}
	s.readings[deviceID] = append(s.readings[deviceID], r)
	return r, nil
}

// ListReadings returns a copy so callers cannot touch the internal slice.
func (s *Store) ListReadings(_ context.Context, deviceID string) ([]store.Reading, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]store.Reading(nil), s.readings[deviceID]...), nil
}
