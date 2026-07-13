// Package config loads runtime configuration from the environment. All
// configuration is env-based so the same binary works in dev, Docker, and on
// PCT 102 without config files.
package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration. See docs/CLAUDE.md for the invariants
// each field participates in (notably: STACKS_DIR must resolve to the same path
// inside and outside the container).
type Config struct {
	Port         string     // PORT — HTTP listen port
	StacksDir    string     // STACKS_DIR — directory scanned for compose stacks
	DataDir      string     // DATA_DIR — SQLite + app state live here
	AuthDisabled bool       // AUTH_DISABLED — bypass auth (LAN test envs only)
	PublicHost   string     // PUBLIC_HOST — host:port used to build homepage URLs (default: request Host)
	LogLevel     slog.Level // LOG_LEVEL — debug|info|warn|error

	CheckInterval time.Duration // CHECK_INTERVAL — periodic update check cadence (0 disables)
}

// Load reads configuration from the environment, applying defaults suitable for
// local development (`task dev`).
func Load() Config {
	return Config{
		Port:         env("PORT", "5001"),
		StacksDir:    env("STACKS_DIR", "./dev-stacks"),
		DataDir:      env("DATA_DIR", "./data"),
		AuthDisabled: envBool("AUTH_DISABLED", false),
		PublicHost:   env("PUBLIC_HOST", ""),
		LogLevel:     logLevel(env("LOG_LEVEL", "info")),

		CheckInterval: envDuration("CHECK_INTERVAL", 30*time.Minute),
	}
}

// envDuration parses a Go duration (e.g. "6h", "30m"); "0"/"off" disables.
func envDuration(key string, fallback time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	if strings.EqualFold(v, "off") {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func logLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
