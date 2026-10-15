//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/laodao-ai/demo-temp-monitor/internal/httpapi"
	"github.com/laodao-ai/demo-temp-monitor/internal/redact"
	"github.com/laodao-ai/demo-temp-monitor/internal/service"
	"github.com/laodao-ai/demo-temp-monitor/internal/store/pgstore"
	"github.com/laodao-ai/demo-temp-monitor/internal/store/storetest"
	"github.com/laodao-ai/demo-temp-monitor/internal/testdb"
)

// TestIntegrationEndToEnd runs TST-08 over HTTP handler + service + PostgreSQL:
// register, report above the threshold, find the reading, see the alarm in the
// log, deregister, find no readings.
func TestIntegrationEndToEnd(t *testing.T) {
	pool := testdb.Open(t)
	st := pgstore.New(pool)

	// The log buffer goes through the redacting handler, like the real service.
	var logBuf bytes.Buffer
	logger := slog.New(redact.Handler(slog.NewTextHandler(&logBuf, nil), pool.Config().ConnString()))
	const thresholdC = 50.0
	srv := httptest.NewServer(httpapi.NewHandler(service.New(st, thresholdC, logger), logger))
	t.Cleanup(srv.Close)

	id := storetest.NewID(t)
	t.Cleanup(func() {
		// Safety net in case a step failed before the deregister step.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, `DELETE FROM devices WHERE id = $1`, id)
	})

	// 1. Register.
	resp := send(t, srv, http.MethodPost, "/devices", `{"id":"`+id+`"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d, want 201", resp.StatusCode)
	}

	// 2. Report a reading above the threshold.
	resp = send(t, srv, http.MethodPost, "/devices/"+id+"/readings", `{"temperature_c":55.0}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("report status = %d, want 201", resp.StatusCode)
	}
	var reported struct {
		ID           int64   `json:"id"`
		DeviceID     string  `json:"device_id"`
		TemperatureC float64 `json:"temperature_c"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reported); err != nil {
		t.Fatal(err)
	}

	// 3. The reading is in the database.
	readings, err := st.ListReadings(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(readings) != 1 || readings[0].ID != reported.ID || readings[0].TemperatureC != 55.0 {
		t.Fatalf("stored readings = %+v, reported = %+v", readings, reported)
	}

	// 4. The log has one alarm for this device.
	got := alarmLines(logBuf.String(), id)
	if len(got) != 1 {
		t.Fatalf("alarm lines for %s = %d, want 1\nlog:\n%s", id, len(got), logBuf.String())
	}
	// TextHandler prints float64 55.0 as 55. Match whole fields so that
	// temperature_c=55 does not also match temperature_c=550.
	fields := strings.Fields(got[0])
	for _, want := range []string{"temperature_c=55", "threshold_c=50"} {
		if !slices.Contains(fields, want) {
			t.Errorf("alarm line lacks field %s: %s", want, got[0])
		}
	}

	// 5. Deregister: the device's readings are gone.
	resp = send(t, srv, http.MethodDelete, "/devices/"+id, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("deregister status = %d, want 204", resp.StatusCode)
	}
	readings, err = st.ListReadings(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(readings) != 0 {
		t.Fatalf("readings after deregister = %d, want 0", len(readings))
	}
}

func send(t *testing.T, srv *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// alarmLines returns the WARN alarm lines that mention this device.
func alarmLines(log, deviceID string) []string {
	var out []string
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, "level=WARN") &&
			strings.Contains(line, `msg="over-temperature alarm"`) &&
			strings.Contains(line, "device_id="+deviceID) {
			out = append(out, line)
		}
	}
	return out
}
