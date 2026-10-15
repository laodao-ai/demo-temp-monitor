package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/laodao-ai/demo-temp-monitor/internal/httpapi"
	"github.com/laodao-ai/demo-temp-monitor/internal/redact"
	"github.com/laodao-ai/demo-temp-monitor/internal/service"
	"github.com/laodao-ai/demo-temp-monitor/internal/store"
	"github.com/laodao-ai/demo-temp-monitor/internal/store/memstore"
)

const maxBody = 1 << 20

const (
	testPassword = "s3cret-xyz"
	testConnURL  = "postgres://monitor:" + testPassword + "@db.internal:7432/temp_monitor_demo"
)

const invalidIDMsg = "invalid device id: must match ^[a-z0-9-]{1,64}$"

type env struct {
	h    http.Handler
	ms   *memstore.Store
	logs *bytes.Buffer
}

func newEnv(t *testing.T, threshold float64) env {
	t.Helper()
	ms := memstore.New()
	return newEnvWith(t, threshold, ms, ms)
}

// newEnvWith serves st over HTTP; ms is kept for direct inspection.
func newEnvWith(t *testing.T, threshold float64, st store.Store, ms *memstore.Store) env {
	t.Helper()
	return newEnvConn(t, threshold, st, ms, testConnURL)
}

// newEnvConn is newEnvWith with the connection string the redactor is told about.
func newEnvConn(t *testing.T, threshold float64, st store.Store, ms *memstore.Store, connURL string) env {
	t.Helper()
	logs := &bytes.Buffer{}
	// The same wrapper main installs in production (Task 3): httpapi relies
	// on the injected logger to redact connection details.
	logger := slog.New(redact.Handler(slog.NewJSONHandler(logs, nil), connURL))
	svc := service.New(st, threshold, logger)
	return env{h: httpapi.NewHandler(svc, logger), ms: ms, logs: logs}
}

func (e env) do(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e env) register(t *testing.T, id string) {
	t.Helper()
	if rec := e.do("POST", "/devices", `{"id":"`+id+`"}`); rec.Code != 201 {
		t.Fatalf("register %s: status %d body %s", id, rec.Code, rec.Body)
	}
}

func (e env) readings(t *testing.T, id string) int {
	t.Helper()
	got, err := e.ms.ListReadings(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return len(got)
}

// errorBody asserts the response is an error with the exact message.
func errorBody(t *testing.T, rec *httptest.ResponseRecorder, wantCode int, wantMsg string) {
	t.Helper()
	if rec.Code != wantCode {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, wantCode, rec.Body)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not a JSON object of strings: %q", rec.Body)
	}
	if len(got) != 1 || got["error"] != wantMsg {
		t.Fatalf("body = %v, want only error=%q", got, wantMsg)
	}
}

// logLines returns the parsed JSON log records.
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		out = append(out, rec)
	}
	return out
}

func alarmCount(t *testing.T, buf *bytes.Buffer) int {
	n := 0
	for _, rec := range logLines(t, buf) {
		if rec["msg"] == "over-temperature alarm" {
			n++
		}
	}
	return n
}

// ---- DEV-01 ----

func TestRegisterCreatesDevice(t *testing.T) {
	e := newEnv(t, 50)
	rec := e.do("POST", "/devices", `{"id": "sensor-01"}`)
	if rec.Code != 201 {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	var got struct {
		ID           string `json:"id"`
		RegisteredAt string `json:"registered_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "sensor-01" {
		t.Errorf("id = %q", got.ID)
	}
	if _, err := time.Parse(time.RFC3339, got.RegisteredAt); err != nil {
		t.Errorf("registered_at %q is not RFC 3339: %v", got.RegisteredAt, err)
	}
	if rec := e.do("POST", "/devices/sensor-01/readings", `{"temperature_c": 20}`); rec.Code != 201 {
		t.Errorf("report after register: status %d", rec.Code)
	}
}

func TestRegisterDuplicateKeepsReadings(t *testing.T) {
	e := newEnv(t, 50)
	e.register(t, "sensor-01")
	e.do("POST", "/devices/sensor-01/readings", `{"temperature_c": 20}`)
	errorBody(t, e.do("POST", "/devices", `{"id":"sensor-01"}`), 409, "device already registered")
	if n := e.readings(t, "sensor-01"); n != 1 {
		t.Errorf("readings = %d, want 1", n)
	}
}

func TestRegisterRejectsBadRequests(t *testing.T) {
	cases := []struct {
		name, body, msg string
	}{
		{"uppercase and underscore", `{"id":"Sensor_01"}`, invalidIDMsg},
		{"empty id", `{"id":""}`, invalidIDMsg},
		{"65 chars", `{"id":"` + strings.Repeat("a", 65) + `"}`, invalidIDMsg},
		{"id is a number", `{"id":5}`, invalidIDMsg},
		{"missing id", `{}`, "id is required"},
		{"null id", `{"id":null}`, "id is required"},
		{"not JSON", `not json`, "invalid JSON body"},
		{"empty body", ``, "invalid JSON body"},
		{"whitespace only body", " \n ", "invalid JSON body"},
		{"top-level array", `[]`, "invalid JSON body"},
		{"trailing content", `{"id":"sensor-01"}garbage`, "invalid JSON body"},
		{"two objects", `{"id":"sensor-01"}{"id":"sensor-02"}`, "invalid JSON body"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, 50)
			errorBody(t, e.do("POST", "/devices", c.body), 400, c.msg)
		})
	}
}

func TestRegisterRejectsBadRequestsWithoutCreating(t *testing.T) {
	e := newEnv(t, 50)
	e.do("POST", "/devices", `{"id":"Sensor_01"}`)
	e.do("POST", "/devices", `{}`)
	// A valid ID in a body that is rejected as a whole must not register it.
	e.do("POST", "/devices", `{"id":"sensor-01"}garbage`)
	// If a device had been created, DELETE would return 204 instead of 404.
	errorBody(t, e.do("DELETE", "/devices/sensor-01", ""), 404, "device not found")
}

func TestIgnoresUnknownFieldsAndContentType(t *testing.T) {
	cases := []struct {
		name, path, body string
		preRegister      bool
	}{
		{"register", "/devices", `{"id":"sensor-01","color":"red"}`, false},
		{"report", "/devices/sensor-01/readings", `{"temperature_c": 20, "unit": "C"}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, 50)
			if c.preRegister {
				e.register(t, "sensor-01")
			}
			req := httptest.NewRequest("POST", c.path, strings.NewReader(c.body))
			req.Header.Set("Content-Type", "text/plain")
			rec := httptest.NewRecorder()
			e.h.ServeHTTP(rec, req)
			if rec.Code != 201 {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestRegisterBodyTooLarge(t *testing.T) {
	e := newEnv(t, 50)
	big := `{"id":"sensor-01","pad":"` + strings.Repeat("x", maxBody) + `"}`
	errorBody(t, e.do("POST", "/devices", big), 400, "request body too large")
}

// ---- DEV-02 / DEV-03 ----

func TestDeleteRemovesDeviceAndReadings(t *testing.T) {
	e := newEnv(t, 50)
	e.register(t, "sensor-01")
	for i := 0; i < 3; i++ {
		e.do("POST", "/devices/sensor-01/readings", `{"temperature_c": 20}`)
	}
	rec := e.do("DELETE", "/devices/sensor-01", "")
	if rec.Code != 204 || rec.Body.Len() != 0 {
		t.Fatalf("status = %d, body %q, want 204 and empty body", rec.Code, rec.Body)
	}
	if n := e.readings(t, "sensor-01"); n != 0 {
		t.Errorf("readings = %d, want 0", n)
	}
	errorBody(t, e.do("POST", "/devices/sensor-01/readings", `{"temperature_c": 20}`), 404, "device not found")
}

func TestDeleteUnregistered(t *testing.T) {
	e := newEnv(t, 50)
	errorBody(t, e.do("DELETE", "/devices/ghost-01", ""), 404, "device not found")
}

func TestDeleteInvalidID(t *testing.T) {
	e := newEnv(t, 50)
	errorBody(t, e.do("DELETE", "/devices/BAD_ID", ""), 400, invalidIDMsg)
}

func TestReRegisterAfterDeleteHasNoReadings(t *testing.T) {
	e := newEnv(t, 50)
	e.register(t, "sensor-01")
	e.do("POST", "/devices/sensor-01/readings", `{"temperature_c": 20}`)
	if rec := e.do("DELETE", "/devices/sensor-01", ""); rec.Code != 204 {
		t.Fatalf("delete status = %d", rec.Code)
	}
	e.register(t, "sensor-01")
	if n := e.readings(t, "sensor-01"); n != 0 {
		t.Errorf("readings = %d, want 0", n)
	}
}

// ---- ING-01 ----

func TestReportStoresReading(t *testing.T) {
	e := newEnv(t, 50)
	e.register(t, "sensor-01")
	rec := e.do("POST", "/devices/sensor-01/readings", `{"temperature_c": 23.5}`)
	if rec.Code != 201 {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got struct {
		ID           int64   `json:"id"`
		DeviceID     string  `json:"device_id"`
		TemperatureC float64 `json:"temperature_c"`
		RecordedAt   string  `json:"recorded_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID == 0 || got.DeviceID != "sensor-01" || got.TemperatureC != 23.5 {
		t.Errorf("body = %+v", got)
	}
	if _, err := time.Parse(time.RFC3339, got.RecordedAt); err != nil {
		t.Errorf("recorded_at %q is not RFC 3339: %v", got.RecordedAt, err)
	}
	if n := e.readings(t, "sensor-01"); n != 1 {
		t.Errorf("readings = %d, want 1", n)
	}
}

func TestReportZeroAndNegativeAreValid(t *testing.T) {
	e := newEnv(t, 50)
	e.register(t, "sensor-01")
	for _, body := range []string{`{"temperature_c": 0}`, `{"temperature_c": -12.5}`} {
		if rec := e.do("POST", "/devices/sensor-01/readings", body); rec.Code != 201 {
			t.Errorf("%s: status = %d, body %s", body, rec.Code, rec.Body)
		}
	}
	got, _ := e.ms.ListReadings(context.Background(), "sensor-01")
	if len(got) != 2 || got[0].TemperatureC != 0 || got[1].TemperatureC != -12.5 {
		t.Errorf("stored = %+v", got)
	}
}

func TestReportAllowsTrailingWhitespace(t *testing.T) {
	e := newEnv(t, 50)
	e.register(t, "sensor-01")
	if rec := e.do("POST", "/devices/sensor-01/readings", "{\"temperature_c\": 20}\n  \n"); rec.Code != 201 {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

// ---- ING-02 ----

func TestReportUnregisteredStoresNothing(t *testing.T) {
	e := newEnv(t, 50)
	errorBody(t, e.do("POST", "/devices/ghost-01/readings", `{"temperature_c": 20}`), 404, "device not found")
	if n := e.readings(t, "ghost-01"); n != 0 {
		t.Errorf("readings = %d, want 0", n)
	}
}

func TestConcurrentDeleteAndReportLeavesNoReadings(t *testing.T) {
	for i := 0; i < 50; i++ {
		e := newEnv(t, 50)
		e.register(t, "sensor-01")
		var wg sync.WaitGroup
		var reportCode, deleteCode int
		wg.Add(2)
		go func() {
			defer wg.Done()
			reportCode = e.do("POST", "/devices/sensor-01/readings", `{"temperature_c": 20}`).Code
		}()
		go func() { defer wg.Done(); deleteCode = e.do("DELETE", "/devices/sensor-01", "").Code }()
		wg.Wait()
		if reportCode != 201 && reportCode != 404 {
			t.Fatalf("report status = %d, want 201 or 404", reportCode)
		}
		if deleteCode != 204 {
			t.Fatalf("delete status = %d, want 204", deleteCode)
		}
		if n := e.readings(t, "sensor-01"); n != 0 {
			t.Fatalf("readings left = %d", n)
		}
	}
}

// ---- ING-03 ----

func TestReportRejectsBadRequests(t *testing.T) {
	cases := []struct {
		name, body, msg string
	}{
		{"missing temperature_c", `{"temp": 23.5}`, "temperature_c is required"},
		{"null temperature_c", `{"temperature_c": null}`, "temperature_c is required"},
		{"string temperature_c", `{"temperature_c": "hot"}`, "temperature_c must be a number"},
		{"bool temperature_c", `{"temperature_c": true}`, "temperature_c must be a number"},
		{"number out of float64 range", `{"temperature_c": 1e999}`, "temperature_c must be a number"},
		{"not JSON", `nope`, "invalid JSON body"},
		{"empty body", ``, "invalid JSON body"},
		{"trailing content", `{"temperature_c": 23.5}garbage`, "invalid JSON body"},
		{"top-level number", `23.5`, "invalid JSON body"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, 50)
			e.register(t, "sensor-01")
			errorBody(t, e.do("POST", "/devices/sensor-01/readings", c.body), 400, c.msg)
			if n := e.readings(t, "sensor-01"); n != 0 {
				t.Errorf("readings = %d, want 0", n)
			}
		})
	}
}

func TestReportInvalidPathID(t *testing.T) {
	e := newEnv(t, 50)
	errorBody(t, e.do("POST", "/devices/BAD_ID/readings", `{"temperature_c": 20}`), 400, invalidIDMsg)
}

// The path ID is checked before the body is read: a bad ID with a broken or
// oversized body gets the invalid-ID error, not a body error.
func TestReportInvalidPathIDWinsOverBadBody(t *testing.T) {
	e := newEnv(t, 50)
	errorBody(t, e.do("POST", "/devices/BAD_ID/readings", `not json`), 400, invalidIDMsg)
	big := `{"temperature_c": 20, "pad": "` + strings.Repeat("x", maxBody) + `"}`
	errorBody(t, e.do("POST", "/devices/BAD_ID/readings", big), 400, invalidIDMsg)
}

func TestReportBodyTooLarge(t *testing.T) {
	const core = `{"temperature_c": 20}`
	cases := []struct{ name, body string }{
		{"single value", `{"temperature_c": 20, "pad": "` + strings.Repeat("x", maxBody) + `"}`},
		{"trailing whitespace", core + strings.Repeat(" ", maxBody)},
		{"one byte over the limit", core + strings.Repeat(" ", maxBody+1-len(core))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, 50)
			e.register(t, "sensor-01")
			errorBody(t, e.do("POST", "/devices/sensor-01/readings", c.body), 400, "request body too large")
			if n := e.readings(t, "sensor-01"); n != 0 {
				t.Errorf("readings = %d, want 0", n)
			}
		})
	}
}

func TestReportBodyExactlyOneMiBIsAccepted(t *testing.T) {
	e := newEnv(t, 50)
	e.register(t, "sensor-01")
	const core = `{"temperature_c": 20}`
	body := core + strings.Repeat(" ", maxBody-len(core))
	if len(body) != maxBody {
		t.Fatalf("test body is %d bytes", len(body))
	}
	if rec := e.do("POST", "/devices/sensor-01/readings", body); rec.Code != 201 {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

// ---- ALM through HTTP ----

func TestAlarmOnlyForSuccessfulOverThresholdReports(t *testing.T) {
	e := newEnv(t, 50)
	e.register(t, "sensor-01")
	e.do("POST", "/devices/sensor-01/readings", `{"temperature_c": 50}`)
	e.do("POST", "/devices/ghost-01/readings", `{"temperature_c": 80}`)
	e.do("POST", "/devices/sensor-01/readings", `{"temperature_c": "80"}`)
	if n := alarmCount(t, e.logs); n != 0 {
		t.Fatalf("alarms = %d, want 0", n)
	}
	rec := e.do("POST", "/devices/sensor-01/readings", `{"temperature_c": 55.0}`)
	if rec.Code != 201 {
		t.Fatalf("status = %d", rec.Code)
	}
	if n := alarmCount(t, e.logs); n != 1 {
		t.Fatalf("alarms = %d, want 1", n)
	}
}

// ---- 5xx and RUN-04 ----

// brokenStore returns an error that carries the connection string and
// password, to prove they reach neither the response body nor the log.
type brokenStore struct{ store.Store }

var errBoom = errors.New("boom: dial " + testConnURL + " failed, password " + testPassword)

func (brokenStore) CreateDevice(context.Context, string) (store.Device, error) {
	return store.Device{}, errBoom
}
func (brokenStore) DeleteDevice(context.Context, string) error { return errBoom }
func (brokenStore) AddReading(context.Context, string, float64) (store.Reading, error) {
	return store.Reading{}, errBoom
}

func TestStoreErrorsGive500WithFixedBodyAndErrorLog(t *testing.T) {
	cases := []struct {
		name, method, path, body, route, deviceID string
	}{
		{"register", "POST", "/devices", `{"id":"sensor-01"}`, "POST /devices", "sensor-01"},
		{"delete", "DELETE", "/devices/sensor-01", "", "DELETE /devices/{id}", "sensor-01"},
		{"report", "POST", "/devices/sensor-01/readings", `{"temperature_c": 80}`, "POST /devices/{id}/readings", "sensor-01"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ms := memstore.New()
			e := newEnvWith(t, 50, brokenStore{ms}, ms)
			rec := e.do(c.method, c.path, c.body)
			errorBody(t, rec, 500, "internal error")
			if strings.Contains(rec.Body.String(), "boom") {
				t.Error("response leaks the underlying error")
			}
			for _, secret := range []string{testPassword, testConnURL} {
				if strings.Contains(e.logs.String(), secret) {
					t.Errorf("log leaks %q: %s", secret, e.logs)
				}
			}
			var found map[string]any
			for _, l := range logLines(t, e.logs) {
				if l["level"] == "ERROR" {
					found = l
				}
			}
			if found == nil {
				t.Fatalf("no ERROR log; logs: %s", e.logs)
			}
			if found["route"] != c.route || found["device_id"] != c.deviceID {
				t.Errorf("ERROR record = %v, want route=%q device_id=%q", found, c.route, c.deviceID)
			}
			if n := alarmCount(t, e.logs); n != 0 {
				t.Errorf("alarms = %d, want 0", n)
			}
		})
	}
}

// A connection string without an extractable password cannot be redacted
// reliably, so the log must carry the fixed text instead of the raw error.
func TestStoreErrorWithoutExtractablePasswordIsWithheldInLog(t *testing.T) {
	const noPasswordURL = "postgres://user@h/db"
	ms := memstore.New()
	e := newEnvConn(t, 50, brokenStore{ms}, ms, noPasswordURL)
	rec := e.do("POST", "/devices", `{"id":"sensor-01"}`)
	errorBody(t, rec, 500, "internal error")
	if strings.Contains(e.logs.String(), "boom") {
		t.Errorf("log carries the raw error text: %s", e.logs)
	}
	var found map[string]any
	for _, l := range logLines(t, e.logs) {
		if l["level"] == "ERROR" {
			found = l
		}
	}
	if found == nil {
		t.Fatalf("no ERROR log; logs: %s", e.logs)
	}
	if found["error"] != "error details withheld" {
		t.Errorf("error attr = %v, want %q", found["error"], "error details withheld")
	}
}

// deadlineStore records the context deadline it was called with.
type deadlineStore struct {
	store.Store
	deadline time.Time
	ok       bool
}

func (d *deadlineStore) AddReading(ctx context.Context, id string, t float64) (store.Reading, error) {
	d.deadline, d.ok = ctx.Deadline()
	return d.Store.AddReading(ctx, id, t)
}

func TestStoreCallsCarryFiveSecondDeadline(t *testing.T) {
	ms := memstore.New()
	ds := &deadlineStore{Store: ms}
	e := newEnvWith(t, 50, ds, ms)
	e.register(t, "sensor-01")
	before := time.Now()
	if rec := e.do("POST", "/devices/sensor-01/readings", `{"temperature_c": 20}`); rec.Code != 201 {
		t.Fatalf("status = %d", rec.Code)
	}
	after := time.Now()
	if !ds.ok {
		t.Fatal("store context has no deadline")
	}
	// The deadline is set once, between before and after, as now+5s. Bracketing
	// it needs no tolerance, so a slow machine cannot fail this.
	if ds.deadline.Before(before.Add(5*time.Second)) || ds.deadline.After(after.Add(5*time.Second)) {
		t.Errorf("deadline %v is outside [%v, %v] (request start + 5s)",
			ds.deadline, before.Add(5*time.Second), after.Add(5*time.Second))
	}
}

func TestStoreContextFollowsRequestCancellation(t *testing.T) {
	ms := memstore.New()
	e := newEnvWith(t, 50, ctxStore{ms}, ms)
	e.register(t, "sensor-01")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("POST", "/devices/sensor-01/readings", strings.NewReader(`{"temperature_c": 20}`)).WithContext(ctx)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	errorBody(t, rec, 500, "internal error")
}

// ctxStore fails when the context is already done, like a real driver would.
type ctxStore struct{ store.Store }

func (c ctxStore) AddReading(ctx context.Context, id string, t float64) (store.Reading, error) {
	if err := ctx.Err(); err != nil {
		return store.Reading{}, err
	}
	return c.Store.AddReading(ctx, id, t)
}
