// Package store defines the storage interface shared by the in-memory and
// PostgreSQL implementations.
package store

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound means the device does not exist.
	ErrNotFound = errors.New("device not found")
	// ErrAlreadyExists means the device ID is already registered.
	ErrAlreadyExists = errors.New("device already registered")
)

// Device is a registered temperature sensor.
type Device struct {
	ID           string
	RegisteredAt time.Time
}

// Reading is one temperature report from a device.
type Reading struct {
	ID           int64
	DeviceID     string
	TemperatureC float64
	RecordedAt   time.Time
}

// Store is the only thing business logic depends on.
type Store interface {
	// CreateDevice registers a device. Already registered -> ErrAlreadyExists.
	CreateDevice(ctx context.Context, id string) (Device, error)
	// DeleteDevice removes a device and its readings. Not registered -> ErrNotFound.
	DeleteDevice(ctx context.Context, id string) error
	// AddReading stores a reading. Device not registered -> ErrNotFound.
	AddReading(ctx context.Context, deviceID string, tempC float64) (Reading, error)
	// ListReadings returns one device's readings in ascending ID order.
	// It exists for contract tests only and is not exposed over HTTP.
	ListReadings(ctx context.Context, deviceID string) ([]Reading, error)
}
