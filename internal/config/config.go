// Package config loads runtime configuration from the environment. All
// configuration is env-based so the same binary works in dev, Docker, and on
// PCT 102 without config files.
package config

import (
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration. See docs/CLAUDE.md for the invariants
// each field participates in (notably: STACKS_DIR must resolve to the same path
// inside and outside the container).
type Config struct {
	Port       string     // PORT — HTTP listen port
	StacksDir  string     // STACKS_DIR — directory scanned for compose stacks
	DataDir    string     // DATA_DIR — SQLite + app state live here
	PublicHost string     // PUBLIC_HOST — host:port used to build homepage URLs (default: request Host)
	LogLevel   slog.Level // LOG_LEVEL — debug|info|warn|error

	// Trusted-header (forward-auth) SSO. When TrustedHeader is set and the direct
	// TCP peer is inside one of TrustedProxyCIDRs, the header's value authenticates
	// the request as admin — the supported replacement for the removed AUTH_DISABLED.
	TrustedHeader     string       // AUTH_TRUSTED_HEADER — e.g. "Remote-User" ("" disables)
	TrustedProxyCIDRs []*net.IPNet // AUTH_TRUSTED_PROXY_CIDRS — comma-separated CIDRs

	// Non-interactive first-run admin bootstrap (CI/scripted installs). Consumed
	// only when no admin exists yet, then ignored. Replaces the dev use of the
	// removed AUTH_DISABLED.
	AdminUser         string // ADMIN_USER — bootstrap admin username
	AdminPasswordFile string // ADMIN_PASSWORD_FILE — path to a file holding the password

	CheckInterval time.Duration // CHECK_INTERVAL — periodic update check cadence (0 disables)
}

// Load reads configuration from the environment, applying defaults suitable for
// local development (`task dev`).
func Load() Config {
	return Config{
		Port:       env("PORT", "5001"),
		StacksDir:  env("STACKS_DIR", "./dev-stacks"),
		DataDir:    env("DATA_DIR", "./data"),
		PublicHost: env("PUBLIC_HOST", ""),
		LogLevel:   logLevel(env("LOG_LEVEL", "info")),

		TrustedHeader:     strings.TrimSpace(env("AUTH_TRUSTED_HEADER", "")),
		TrustedProxyCIDRs: parseCIDRs(env("AUTH_TRUSTED_PROXY_CIDRS", "")),

		AdminUser:         strings.TrimSpace(env("ADMIN_USER", "")),
		AdminPasswordFile: strings.TrimSpace(env("ADMIN_PASSWORD_FILE", "")),

		CheckInterval: envDuration("CHECK_INTERVAL", 30*time.Minute),
	}
}

// AuthDisabledRemoved reports whether the retired AUTH_DISABLED variable is set
// (present, ok) and whether its value is truthy. main refuses to boot on a
// truthy value so the security change is impossible to miss.
func AuthDisabledRemoved() (present, truthy bool) {
	v, ok := os.LookupEnv("AUTH_DISABLED")
	if !ok {
		return false, false
	}
	b, _ := strconv.ParseBool(strings.TrimSpace(v))
	return true, b
}

// parseCIDRs parses a comma-separated CIDR list; unparseable entries are
// dropped (they simply never match — fail closed).
func parseCIDRs(s string) []*net.IPNet {
	var out []*net.IPNet
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(part); err == nil {
			out = append(out, n)
		}
	}
	return out
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
