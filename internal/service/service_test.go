package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/laodao-ai/demo-temp-monitor/internal/service"
	"github.com/laodao-ai/demo-temp-monitor/internal/store"
	"github.com/laodao-ai/demo-temp-monitor/internal/store/memstore"
)

// logRecord is one parsed slog JSON line.
type logRecord struct {
	Level        string  `json:"level"`
	Msg          string  `json:"msg"`
	DeviceID     string  `json:"device_id"`
	TemperatureC float64 `json:"temperature_c"`
	ThresholdC   float64 `json:"threshold_c"`
}

// alarms returns the records in buf whose msg is the alarm message.
func alarms(t *testing.T, buf *bytes.Buffer) []logRecord {
	t.Helper()
	var out []logRecord
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec logRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %q: %v", line, err)
		}
		if rec.Msg == "over-temperature alarm" {
			out = append(out, rec)
		}
	}
	return out
}

func newSvc(t *testing.T, threshold float64, s store.Store) (*service.Service, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))
	return service.New(s, threshold, logger), buf
}

// failingStore fails every reading write; other calls go to the wrapped store.
type failingStore struct{ store.Store }

func (failingStore) AddReading(context.Context, string, float64) (store.Reading, error) {
	return store.Reading{}, errors.New("disk on fire")
}

func TestRegisterValidatesID(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(t, 50, memstore.New())

	invalid := []string{"Sensor_01", "", strings.Repeat("a", 65), "a b", "sensor-01\n", "传感器"}
	for _, id := range invalid {
		if _, err := svc.Register(ctx, id); !errors.Is(err, service.ErrInvalidID) {
			t.Errorf("Register(%q) error = %v, want ErrInvalidID", id, err)
		}
	}
	for _, id := range []string{"sensor-01", "a", strings.Repeat("a", 64), "0-9"} {
		if _, err := svc.Register(ctx, id); err != nil {
			t.Errorf("Register(%q) error = %v, want nil", id, err)
		}
	}
}

func TestRegisterDuplicate(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(t, 50, memstore.New())
	if _, err := svc.Register(ctx, "sensor-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Register(ctx, "sensor-01"); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("second Register error = %v, want ErrAlreadyExists", err)
	}
}

func TestDeregister(t *testing.T) {
	ctx := context.Background()
	ms := memstore.New()
	svc, _ := newSvc(t, 50, ms)

	if err := svc.Deregister(ctx, "BAD_ID"); !errors.Is(err, service.ErrInvalidID) {
		t.Errorf("Deregister(BAD_ID) error = %v, want ErrInvalidID", err)
	}
	if err := svc.Deregister(ctx, "ghost-01"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Deregister(ghost-01) error = %v, want ErrNotFound", err)
	}

	// DEV-02 / DEV-03: readings go away, reports are rejected, re-register starts empty.
	if _, err := svc.Register(ctx, "sensor-01"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := svc.Report(ctx, "sensor-01", 20); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Deregister(ctx, "sensor-01"); err != nil {
		t.Fatal(err)
	}
	if got, _ := ms.ListReadings(ctx, "sensor-01"); len(got) != 0 {
		t.Errorf("readings after deregister = %d, want 0", len(got))
	}
	if _, err := svc.Report(ctx, "sensor-01", 20); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Report after deregister error = %v, want ErrNotFound", err)
	}
	if _, err := svc.Register(ctx, "sensor-01"); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if got, _ := ms.ListReadings(ctx, "sensor-01"); len(got) != 0 {
		t.Errorf("readings after re-register = %d, want 0", len(got))
	}
}

func TestReportStoresReading(t *testing.T) {
	ctx := context.Background()
	ms := memstore.New()
	svc, _ := newSvc(t, 50, ms)
	if _, err := svc.Register(ctx, "sensor-01"); err != nil {
		t.Fatal(err)
	}
	for _, temp := range []float64{23.5, 0, -12.5} {
		r, err := svc.Report(ctx, "sensor-01", temp)
		if err != nil {
			t.Fatalf("Report(%v): %v", temp, err)
		}
		if r.DeviceID != "sensor-01" || r.TemperatureC != temp {
			t.Errorf("Report(%v) = %+v", temp, r)
		}
	}
	if got, _ := ms.ListReadings(ctx, "sensor-01"); len(got) != 3 {
		t.Errorf("stored readings = %d, want 3", len(got))
	}
}

func TestReportInvalidIDStoresNothing(t *testing.T) {
	svc, buf := newSvc(t, 50, memstore.New())
	if _, err := svc.Report(context.Background(), "BAD_ID", 99); !errors.Is(err, service.ErrInvalidID) {
		t.Fatalf("error = %v, want ErrInvalidID", err)
	}
	if got := alarms(t, buf); len(got) != 0 {
		t.Errorf("alarms = %v, want none", got)
	}
}

func TestAlarmOverThreshold(t *testing.T) {
	ctx := context.Background()
	svc, buf := newSvc(t, 50, memstore.New())
	if _, err := svc.Register(ctx, "sensor-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Report(ctx, "sensor-01", 55.0); err != nil {
		t.Fatal(err)
	}
	got := alarms(t, buf)
	if len(got) != 1 {
		t.Fatalf("alarms = %d, want 1", len(got))
	}
	want := logRecord{Level: "WARN", Msg: "over-temperature alarm", DeviceID: "sensor-01", TemperatureC: 55, ThresholdC: 50}
	if got[0] != want {
		t.Errorf("alarm = %+v, want %+v", got[0], want)
	}
}

func TestAlarmThresholdIsConfigurable(t *testing.T) {
	ctx := context.Background()
	svc, buf := newSvc(t, 30, memstore.New())
	if _, err := svc.Register(ctx, "sensor-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Report(ctx, "sensor-01", 31); err != nil {
		t.Fatal(err)
	}
	got := alarms(t, buf)
	if len(got) != 1 || got[0].ThresholdC != 30 {
		t.Fatalf("alarms = %+v, want one with threshold_c=30", got)
	}
}

func TestNoAlarmAtOrBelowThreshold(t *testing.T) {
	ctx := context.Background()
	svc, buf := newSvc(t, 50, memstore.New())
	if _, err := svc.Register(ctx, "sensor-01"); err != nil {
		t.Fatal(err)
	}
	for _, temp := range []float64{50.0, 23.5} {
		if _, err := svc.Report(ctx, "sensor-01", temp); err != nil {
			t.Fatalf("Report(%v): %v", temp, err)
		}
	}
	if got := alarms(t, buf); len(got) != 0 {
		t.Errorf("alarms = %+v, want none", got)
	}
}

func TestNoAlarmForUnregisteredDevice(t *testing.T) {
	svc, buf := newSvc(t, 50, memstore.New())
	if _, err := svc.Report(context.Background(), "ghost-01", 80); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if got := alarms(t, buf); len(got) != 0 {
		t.Errorf("alarms = %+v, want none", got)
	}
}

func TestNoAlarmWhenStoreFails(t *testing.T) {
	svc, buf := newSvc(t, 50, failingStore{memstore.New()})
	if _, err := svc.Report(context.Background(), "sensor-01", 80); err == nil {
		t.Fatal("want error")
	}
	if got := alarms(t, buf); len(got) != 0 {
		t.Errorf("alarms = %+v, want none", got)
	}
}

func TestEveryOverTemperatureReadingAlarms(t *testing.T) {
	ctx := context.Background()
	svc, buf := newSvc(t, 50, memstore.New())
	if _, err := svc.Register(ctx, "sensor-01"); err != nil {
		t.Fatal(err)
	}
	for _, temp := range []float64{55.0, 56.0} {
		if _, err := svc.Report(ctx, "sensor-01", temp); err != nil {
			t.Fatal(err)
		}
	}
	got := alarms(t, buf)
	if len(got) != 2 || got[0].TemperatureC != 55 || got[1].TemperatureC != 56 {
		t.Fatalf("alarms = %+v, want two with temperature_c 55 then 56", got)
	}
}
