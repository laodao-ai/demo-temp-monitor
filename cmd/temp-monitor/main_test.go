package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/laodao-ai/demo-temp-monitor/internal/store"
	"github.com/laodao-ai/demo-temp-monitor/internal/store/memstore"
	"github.com/laodao-ai/demo-temp-monitor/internal/store/pgstore"
)

// lockedBuffer lets the test read stderr while run is still writing to it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// noFile is a config path that does not exist, so only env values count.
const noFile = "testdata/does-not-exist.env"

// neverOpen fails the test if the database is touched.
func neverOpen(t *testing.T) openFunc {
	return func(context.Context, string) (store.Store, func(), error) {
		t.Fatal("open must not be called")
		return nil, nil, nil
	}
}

const connectLine = "cannot connect to database; check the SSH tunnel and that the URL uses the session pool port 7432\n"

func TestStartupFailuresBeforeDatabase(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			"missing URL",
			map[string]string{},
			"TM_DATABASE_URL is not set; copy config/example.env to config/local.env, fill it in, and start from the repo root\n",
		},
		{
			"threshold not a number",
			map[string]string{"TM_DATABASE_URL": "postgres://u:pw@127.0.0.1:1/d", "TM_ALARM_THRESHOLD_C": "abc"},
			"TM_ALARM_THRESHOLD_C is invalid; must be a finite number, such as 50.0\n",
		},
		{
			"threshold NaN",
			map[string]string{"TM_DATABASE_URL": "postgres://u:pw@127.0.0.1:1/d", "TM_ALARM_THRESHOLD_C": "NaN"},
			"TM_ALARM_THRESHOLD_C is invalid; must be a finite number, such as 50.0\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := run(t.Context(), env(c.env), noFile, &stderr, neverOpen(t))
			if code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}
			if stderr.String() != c.want {
				t.Fatalf("stderr = %q, want %q", stderr.String(), c.want)
			}
		})
	}
}

// Passwords written without URL encoding. The driver can report a piece of
// them as a host name, so the only allowed output is the fixed text.
func TestUnreachableDatabaseNeverPrintsPassword(t *testing.T) {
	passwords := []string{"s3cret-xyz", "p(ss;w", "p&ss", "p%zz", "p@ss#w"}
	for _, pw := range passwords {
		t.Run(pw, func(t *testing.T) {
			url := "postgres://temp_monitor_demo:" + pw + "@127.0.0.1:1/temp_monitor_demo?sslmode=disable&connect_timeout=5"
			var stderr bytes.Buffer
			start := time.Now()
			code := run(t.Context(), env(map[string]string{"TM_DATABASE_URL": url}), noFile, &stderr, openPostgres)
			if code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}
			if stderr.String() != connectLine {
				t.Fatalf("stderr = %q, want %q", stderr.String(), connectLine)
			}
			if elapsed := time.Since(start); elapsed > 10*time.Second {
				t.Fatalf("took %v, want at most 10s", elapsed)
			}
		})
	}
}

func TestOpenFailureMessages(t *testing.T) {
	const url = "postgres://u:s3cret-xyz@127.0.0.1:7432/d?sslmode=disable"
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"connect", pgstore.ErrConnect, connectLine},
		{"missing tables", pgstore.ErrSchemaMissing, "table devices or readings not found; run scripts/db-reset.sh first\n"},
		{
			"other driver error is redacted",
			errors.New("query failed for " + url + ": password s3cret-xyz rejected"),
			"cannot check database schema: query failed for ***: password *** rejected\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			open := func(context.Context, string) (store.Store, func(), error) { return nil, nil, c.err }
			var stderr bytes.Buffer
			code := run(t.Context(), env(map[string]string{"TM_DATABASE_URL": url}), noFile, &stderr, open)
			if code != 1 || stderr.String() != c.want {
				t.Fatalf("code=%d stderr=%q, want code 1 and %q", code, stderr.String(), c.want)
			}
		})
	}
}

// A signal during startup cancels the connect, which the driver reports as
// ErrConnect; the tunnel hint would be wrong, so the fixed text is printed.
func TestInterruptedDuringStartupExitsOne(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	open := func(context.Context, string) (store.Store, func(), error) {
		cancel()
		return nil, nil, pgstore.ErrConnect
	}
	var stderr bytes.Buffer
	code := run(ctx, env(map[string]string{"TM_DATABASE_URL": "postgres://u:pw@h/d"}), noFile, &stderr, open)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if want := "interrupted before the service started\n"; stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestOtherDriverErrorWithoutExtractablePasswordIsWithheld(t *testing.T) {
	const url = "postgres://127.0.0.1:7432/d" // no password in the URL
	open := func(context.Context, string) (store.Store, func(), error) {
		return nil, nil, errors.New("something with details")
	}
	var stderr bytes.Buffer
	code := run(t.Context(), env(map[string]string{"TM_DATABASE_URL": url}), noFile, &stderr, open)
	want := "cannot check database schema: error details withheld\n"
	if code != 1 || stderr.String() != want {
		t.Fatalf("code=%d stderr=%q, want code 1 and %q", code, stderr.String(), want)
	}
}

func memOpen(closed *bool) openFunc {
	return func(context.Context, string) (store.Store, func(), error) {
		return memstore.New(), func() { *closed = true }, nil
	}
}

func TestListenFailureExitsOne(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	addr := busy.Addr().String()

	var closed bool
	var stderr bytes.Buffer
	code := run(t.Context(), env(map[string]string{"TM_DATABASE_URL": "postgres://u:pw@h/d", "TM_LISTEN_ADDR": addr}),
		noFile, &stderr, memOpen(&closed))
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	out := stderr.String()
	if !strings.HasPrefix(out, "cannot listen on "+addr+": ") ||
		!strings.HasSuffix(out, "; the port may be in use; free it or set TM_LISTEN_ADDR\n") {
		t.Fatalf("stderr = %q", out)
	}
	if !closed {
		t.Fatal("store was not closed after the listen failure")
	}
}

// Cancelling ctx stands for SIGINT / SIGTERM: run must log the startup line,
// shut down, close the store, and return 0.
func TestGracefulShutdown(t *testing.T) {
	const url = "postgres://u:s3cret-xyz@h/d"
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var closed bool
	stderr := &lockedBuffer{}
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, env(map[string]string{
			"TM_DATABASE_URL":      url,
			"TM_LISTEN_ADDR":       "127.0.0.1:0",
			"TM_ALARM_THRESHOLD_C": "42.5",
		}), noFile, stderr, memOpen(&closed))
	}()

	waitFor(t, func() bool { return strings.Contains(stderr.String(), "msg=listening") })
	startup := stderr.String()
	if !strings.Contains(startup, "level=INFO") || !strings.Contains(startup, "addr=127.0.0.1:") ||
		!strings.Contains(startup, "threshold_c=42.5") {
		t.Fatalf("startup line = %q", startup)
	}

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0\n%s", code, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return after cancel")
	}
	if !closed {
		t.Fatal("store was not closed on shutdown")
	}
	if all := stderr.String(); strings.Contains(all, "s3cret-xyz") || strings.Contains(all, url) {
		t.Fatalf("log contains the connection string or password:\n%s", all)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 5s")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// blockingStore blocks CreateDevice until its ctx ends and records why.
type blockingStore struct {
	store.Store
	entered chan struct{}
	ended   chan error
}

func (b *blockingStore) CreateDevice(ctx context.Context, _ string) (store.Device, error) {
	close(b.entered)
	<-ctx.Done()
	b.ended <- ctx.Err()
	return store.Device{}, ctx.Err()
}

// [impl-review-fix] The request body is still arriving when shutdown starts, so
// the store call (with its own 5 s timeout) begins after the shutdown wait is
// already running. Closing the server at the 5 s limit must cancel that call
// (context.Canceled) instead of leaving it to its own later deadline (RUN-06).
func TestShutdownTimeoutCancelsInFlightRequest(t *testing.T) {
	bs := &blockingStore{Store: memstore.New(), entered: make(chan struct{}), ended: make(chan error, 1)}
	open := func(context.Context, string) (store.Store, func(), error) { return bs, func() {}, nil }

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // free the port so run can bind it

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stderr := &lockedBuffer{}
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, env(map[string]string{"TM_DATABASE_URL": "postgres://u:pw@h/d", "TM_LISTEN_ADDR": addr}),
			noFile, stderr, open)
	}()
	waitFor(t, func() bool { return strings.Contains(stderr.String(), "msg=listening") })

	// Send the headers and part of the body now, the rest after shutdown began.
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	body := `{"id":"d1"}`
	head := "POST /devices HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: " +
		strconv.Itoa(len(body)) + "\r\n\r\n"
	if _, err := conn.Write([]byte(head + body[:5])); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond) // let the server start handling the request

	start := time.Now()
	cancel()
	time.Sleep(2 * time.Second)
	if _, err := conn.Write([]byte(body[5:])); err != nil {
		t.Fatal(err)
	}
	<-bs.entered // store timeout (5 s) now runs until about start+7s

	select {
	case err := <-bs.ended:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("store ctx ended with %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight store call was not canceled at the shutdown limit")
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0\n%s", code, stderr.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return after the server was closed")
	}
	if !strings.Contains(stderr.String(), "shutdown did not finish in time") {
		t.Fatalf("stderr lacks the shutdown timeout WARN:\n%s", stderr.String())
	}
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Fatalf("shutdown took %v, want about 5s", elapsed)
	}
}

// failingListener accepts nothing: Accept returns a permanent error, which
// makes http.Server.Serve return something other than ErrServerClosed.
type failingListener struct{ net.Listener }

func (failingListener) Accept() (net.Conn, error) { return nil, errors.New("accept broke") }

func TestServeUnexpectedErrorExitsOne(t *testing.T) {
	real, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { real.Close() })
	old := listen
	listen = func(string, string) (net.Listener, error) { return failingListener{real}, nil }
	t.Cleanup(func() { listen = old })

	open := func(context.Context, string) (store.Store, func(), error) { return memstore.New(), func() {}, nil }
	stderr := &lockedBuffer{}
	done := make(chan int, 1)
	go func() {
		done <- run(t.Context(), env(map[string]string{"TM_DATABASE_URL": "postgres://u:pw@h/d"}), noFile, stderr, open)
	}()
	select {
	case code := <-done:
		if code != 1 {
			t.Fatalf("exit code = %d, want 1\n%s", code, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return after Serve failed")
	}
	if all := stderr.String(); !strings.Contains(all, "server stopped unexpectedly") || !strings.Contains(all, "accept broke") {
		t.Fatalf("stderr lacks the unexpected-stop ERROR:\n%s", all)
	}
}
