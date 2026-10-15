// Package config resolves the three service settings.
// Priority: environment variable > config/local.env > default.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"strconv"
	"strings"
)

const (
	KeyDatabaseURL = "TM_DATABASE_URL"
	KeyListenAddr  = "TM_LISTEN_ADDR"
	KeyThreshold   = "TM_ALARM_THRESHOLD_C"

	defaultListenAddr = "127.0.0.1:8080"
	defaultThreshold  = 50.0
)

// Sentinel errors let callers pick a next-step hint with errors.Is instead of
// matching message text. Each error message is fixed text that never contains a
// value; the file-grammar ones are wrapped with "<path> line <n>: ".
var (
	ErrNoURL            = errors.New(KeyDatabaseURL + " is not set")
	ErrThresholdInvalid = errors.New(KeyThreshold + " is invalid")
	ErrMissingEquals    = errors.New(`missing "="`)
	ErrUnknownKey       = errors.New("unknown key")
	ErrQuotedValue      = errors.New("value must not be quoted")
)

// Config holds the three service settings.
type Config struct {
	DatabaseURL     string
	ListenAddr      string
	AlarmThresholdC float64
}

var knownKeys = map[string]bool{KeyDatabaseURL: true, KeyListenAddr: true, KeyThreshold: true}

// Load reads settings from getenv and the env file at path.
// A missing file is not an error. Errors never echo line content or values,
// because the file holds the database password.
func Load(getenv func(string) string, path string) (Config, error) {
	fileVals, err := parseFile(path)
	if err != nil {
		return Config{}, err
	}
	// An empty value, in the environment or the file, counts as unset.
	pick := func(key string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return fileVals[key]
	}

	cfg := Config{
		DatabaseURL:     pick(KeyDatabaseURL),
		ListenAddr:      pick(KeyListenAddr),
		AlarmThresholdC: defaultThreshold,
	}
	if cfg.DatabaseURL == "" {
		return Config{}, ErrNoURL
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultListenAddr
	}
	if raw := pick(KeyThreshold); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return Config{}, ErrThresholdInvalid
		}
		cfg.AlarmThresholdC = v
	}
	return cfg, nil
}

// parseFile applies the line grammar from design "Configuration Surface".
func parseFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		// Append only the errno text (permission denied, is a directory, ...).
		// PathError.Error() would repeat the path, and file content is never
		// part of the error.
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			return nil, fmt.Errorf("%s: cannot read file: %v", path, pathErr.Err)
		}
		return nil, fmt.Errorf("%s: cannot read file", path)
	}

	vals := map[string]string{}
	for i, line := range strings.Split(string(data), "\n") {
		n := i + 1
		line = strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s line %d: %w", path, n, ErrMissingEquals)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if !knownKeys[key] {
			return nil, fmt.Errorf("%s line %d: %w", path, n, ErrUnknownKey)
		}
		if strings.HasPrefix(val, `"`) || strings.HasPrefix(val, `'`) {
			return nil, fmt.Errorf("%s line %d: %w", path, n, ErrQuotedValue)
		}
		vals[key] = val
	}
	return vals, nil
}
