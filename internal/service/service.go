// Package service holds the business rules: ID validation, register,
// deregister, report, and the over-temperature alarm. It depends only on
// store.Store.
package service

import (
	"context"
	"errors"
	"log/slog"
	"regexp"

	"github.com/laodao-ai/demo-temp-monitor/internal/store"
)

// ErrInvalidID means the device ID does not match IDPattern.
var ErrInvalidID = errors.New("invalid device id")

// IDPattern is the device ID format (DEV-01). Go's $ matches only at the end
// of the text, so a trailing newline is rejected.
const IDPattern = `^[a-z0-9-]{1,64}$`

var idRegexp = regexp.MustCompile(IDPattern)

// Service has no mutable state after construction.
type Service struct {
	store      store.Store
	thresholdC float64
	logger     *slog.Logger
}

func New(s store.Store, thresholdC float64, logger *slog.Logger) *Service {
	return &Service{store: s, thresholdC: thresholdC, logger: logger}
}

// ValidateID returns ErrInvalidID when id does not match IDPattern. httpapi
// calls it before reading a request body.
func ValidateID(id string) error {
	if !idRegexp.MatchString(id) {
		return ErrInvalidID
	}
	return nil
}

func (s *Service) Register(ctx context.Context, id string) (store.Device, error) {
	if err := ValidateID(id); err != nil {
		return store.Device{}, err
	}
	return s.store.CreateDevice(ctx, id)
}

func (s *Service) Deregister(ctx context.Context, id string) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	return s.store.DeleteDevice(ctx, id)
}

// Report stores a reading. The alarm is written only after the store call
// succeeded, and only when the temperature is strictly above the threshold.
func (s *Service) Report(ctx context.Context, id string, tempC float64) (store.Reading, error) {
	if err := ValidateID(id); err != nil {
		return store.Reading{}, err
	}
	r, err := s.store.AddReading(ctx, id, tempC)
	if err != nil {
		return store.Reading{}, err
	}
	if tempC > s.thresholdC {
		s.logger.Warn("over-temperature alarm",
			"device_id", id,
			"temperature_c", tempC,
			"threshold_c", s.thresholdC)
	}
	return r, nil
}
