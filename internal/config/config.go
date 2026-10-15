// Package config resolves the three service settings.
// Priority: environment variable > config/local.env > default.
package config

import (
	"errors"
	"fmt"
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
		return Config{}, fmt.Errorf("%s is not set", KeyDatabaseURL)
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultListenAddr
	}
	if raw := pick(KeyThreshold); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return Config{}, fmt.Errorf("%s is invalid", KeyThreshold)
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
			return nil, fmt.Errorf("%s line %d: missing \"=\"", path, n)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if !knownKeys[key] {
			return nil, fmt.Errorf("%s line %d: unknown key", path, n)
		}
		if strings.HasPrefix(val, `"`) || strings.HasPrefix(val, `'`) {
			return nil, fmt.Errorf("%s line %d: value must not be quoted", path, n)
		}
		vals[key] = val
	}
	return vals, nil
}
