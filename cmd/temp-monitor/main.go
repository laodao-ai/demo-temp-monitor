// Command temp-monitor is the temperature monitoring HTTP service.
// Start it from the repo root: it reads config/local.env relative to the
// working directory.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/laodao-ai/demo-temp-monitor/internal/config"
	"github.com/laodao-ai/demo-temp-monitor/internal/httpapi"
	"github.com/laodao-ai/demo-temp-monitor/internal/redact"
	"github.com/laodao-ai/demo-temp-monitor/internal/service"
	"github.com/laodao-ai/demo-temp-monitor/internal/store"
	"github.com/laodao-ai/demo-temp-monitor/internal/store/pgstore"
)

const (
	envFile = "config/local.env"

	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 5 * time.Second

	hintNoURL     = "copy config/example.env to config/local.env, fill it in, and start from the repo root"
	hintThreshold = "must be a finite number, such as 50.0"
	hintConnect   = "check the SSH tunnel and that the URL uses the session pool port 7432"
	hintNoTables  = "run scripts/db-reset.sh first"
	hintListen    = "the port may be in use; free it or set TM_LISTEN_ADDR"
)

// openFunc connects to the database and checks the schema. close releases
// the connections. Tests replace it so no database is needed.
type openFunc func(ctx context.Context, connURL string) (s store.Store, close func(), err error)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Getenv, envFile, os.Stderr, openPostgres)
	stop()
	os.Exit(code)
}

// openPostgres returns pgstore.ErrConnect or pgstore.ErrSchemaMissing for the
// two expected failures; any other error is driver text and must be redacted.
func openPostgres(ctx context.Context, connURL string) (store.Store, func(), error) {
	pool, err := pgstore.Connect(ctx, connURL)
	if err != nil {
		return nil, nil, err
	}
	if err := pgstore.CheckSchema(ctx, pool); err != nil {
		pool.Close()
		return nil, nil, err
	}
	return pgstore.New(pool), pool.Close, nil
}

// run starts the service and returns the process exit code. It returns 0 only
// after a graceful shutdown that was triggered by ctx being canceled.
func run(ctx context.Context, getenv func(string) string, envPath string, stderr io.Writer, open openFunc) int {
	cfg, err := config.Load(getenv, envPath)
	if err != nil {
		// Config errors name keys and line numbers only, never values.
		fail(stderr, err.Error(), configHint(err))
		return 1
	}

	// Every log line goes through the redacting handler built from the real
	// connection string, so a driver error can never print it.
	logger := slog.New(redact.Handler(slog.NewTextHandler(stderr, nil), cfg.DatabaseURL))

	st, closeStore, err := open(ctx, cfg.DatabaseURL)
	if err != nil {
		switch {
		case errors.Is(err, pgstore.ErrConnect):
			// Fixed text only: the driver can echo part of a malformed password.
			fail(stderr, err.Error(), hintConnect)
		case errors.Is(err, pgstore.ErrSchemaMissing):
			fail(stderr, err.Error(), hintNoTables)
		default:
			text, ok := redact.Text(err.Error(), cfg.DatabaseURL)
			if !ok {
				text = "error details withheld"
			}
			fail(stderr, "cannot check database schema: "+text, "")
		}
		return 1
	}
	defer closeStore()

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		fail(stderr, fmt.Sprintf("cannot listen on %s: %v", cfg.ListenAddr, err), hintListen)
		return 1
	}
	logger.Info("listening", "addr", ln.Addr().String(), "threshold_c", cfg.AlarmThresholdC)

	svc := service.New(st, cfg.AlarmThresholdC, logger)
	srv := &http.Server{
		Handler:           httpapi.NewHandler(svc, logger),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	// This goroutine is the signal wait: it turns ctx cancellation into Shutdown.
	serveCtx, stopServe := context.WithCancel(ctx)
	defer stopServe()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-serveCtx.Done()
		logger.Info("shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			logger.Warn("shutdown did not finish in time", "error", err)
			// [impl-review-fix] Close cancels in-flight request contexts (and so
			// their store calls); otherwise closeStore waits for borrowed
			// connections and exit can pass the 5 s limit (RUN-06).
			_ = srv.Close()
		}
	}()

	err = srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		// Serve returns as soon as Shutdown starts; wait for Shutdown itself.
		<-shutdownDone
		logger.Info("stopped")
		return 0
	}
	logger.Error("server stopped unexpectedly", "error", err)
	stopServe()
	<-shutdownDone
	return 1
}

// fail prints "<text>; <hint>" on one line.
func fail(w io.Writer, text, hint string) {
	if hint != "" {
		text += "; " + hint
	}
	fmt.Fprintln(w, text)
}

// configHint picks the next step for a config.Load error. The messages are
// fixed texts from internal/config, so matching on them is safe.
func configHint(err error) string {
	msg := err.Error()
	switch {
	case msg == config.KeyDatabaseURL+" is not set":
		return hintNoURL
	case msg == config.KeyThreshold+" is invalid":
		return hintThreshold
	case strings.HasSuffix(msg, `missing "="`):
		return "use KEY=VALUE"
	case strings.HasSuffix(msg, "unknown key"):
		return "allowed keys: " + config.KeyDatabaseURL + ", " + config.KeyListenAddr + ", " + config.KeyThreshold
	case strings.HasSuffix(msg, "must not be quoted"):
		return "remove the quotes"
	}
	return ""
}
