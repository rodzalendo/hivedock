// Package server wires the HTTP router: REST for request/response, a single
// multiplexed WebSocket for streams, and the embedded SPA as the fallback.
package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/rogalinski/hivedock/internal/compose"
	"github.com/rogalinski/hivedock/internal/config"
	"github.com/rogalinski/hivedock/internal/discovery"
	"github.com/rogalinski/hivedock/internal/docker"
	"github.com/rogalinski/hivedock/internal/events"
	"github.com/rogalinski/hivedock/internal/hoststats"
	"github.com/rogalinski/hivedock/internal/registry"
	"github.com/rogalinski/hivedock/internal/stacks"
	"github.com/rogalinski/hivedock/internal/store"
	"github.com/rogalinski/hivedock/internal/updates"
)

// version is the build-time version string; overridden via -ldflags in the
// Dockerfile. "dev" for local builds.
var version = "dev"

// New builds the top-level HTTP handler. ctx bounds background loops (the
// periodic update scheduler); cancel it to stop them.
func New(ctx context.Context, cfg config.Config, logger *slog.Logger, db *store.Store, stacksSvc *stacks.Manager, hub *events.Hub, host *hoststats.Sampler, dockerClient *docker.Client, icons *discovery.IconResolver, dist fs.FS) http.Handler {
	return newServer(ctx, cfg, logger, db, stacksSvc, hub, host, dockerClient, icons, dist).mux
}

// newServer wires the api and its router, returning the *api so in-package
// tests can reach state like the first-run setup token. Callers outside the
// package use New.
func newServer(ctx context.Context, cfg config.Config, logger *slog.Logger, db *store.Store, stacksSvc *stacks.Manager, hub *events.Hub, host *hoststats.Sampler, dockerClient *docker.Client, icons *discovery.IconResolver, dist fs.FS) *api {
	r := chi.NewRouter()
	// capturePeer must run BEFORE RealIP: trusted-header auth decides trust from
	// the real TCP peer, and RealIP overwrites RemoteAddr from X-Forwarded-For
	// (which is attacker-controllable). Order matters for a security invariant.
	r.Use(capturePeer)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)

	// The update checker uses the Docker client (nil-safe) for local image
	// digests + changelog source labels on the mutable-tag path.
	var local updates.LocalImages
	if dockerClient != nil {
		local = dockerClient
	}
	checker := updates.NewChecker(registry.NewClient(nil), local, logger)

	api := &api{cfg: cfg, logger: logger, db: db, stacks: stacksSvc, hub: hub, host: host, docker: dockerClient, icons: icons, runner: compose.NewRunner(), checker: checker, login: newLoginLimiter()}

	// First-run: bootstrap the admin from env, or mint a one-time setup token.
	api.initFirstRun()

	// Periodic background update checks (settings override CHECK_INTERVAL;
	// cadence changes apply live).
	api.startUpdateScheduler(ctx)

	r.Route("/api", func(r chi.Router) {
		// Public: liveness + the auth bootstrap (status/setup/login).
		r.Get("/health", api.health)
		r.Route("/auth", func(r chi.Router) {
			r.Get("/status", api.authStatus)
			r.Post("/setup", api.authSetup)
			r.Post("/login", api.authLogin)
			r.With(api.requireAuth).Post("/logout", api.authLogout)
		})

		// Everything else requires a session or a trusted-proxy header.
		r.Group(func(r chi.Router) {
			r.Use(api.requireAuth)
			r.Get("/ws", api.websocket)
			r.Get("/stacks", api.listStacks)
			r.Post("/stacks", api.createStack)
			r.Get("/stacks/{name}", api.getStack)
			r.Delete("/stacks/{name}", api.deleteStack)
			r.Post("/stacks/{name}/rename", api.renameStack)
			r.Post("/stacks/{name}/actions/{action}", api.runStackAction)
			r.Post("/stacks/{name}/services/{service}/restart", api.restartService)
			r.Post("/stacks/{name}/services/{service}/update", api.updateService)
			r.Get("/stacks/{name}/compose", api.getCompose)
			r.Put("/stacks/{name}/compose", api.putCompose)
			r.Post("/stacks/{name}/compose/validate", api.validateCompose)
			r.Get("/stacks/{name}/env", api.getEnv)
			r.Put("/stacks/{name}/env", api.putEnv)
			r.Get("/host/stats", api.hostStats)
			r.Post("/system/prune", api.prune)
			r.Get("/settings", api.settings)
			r.Put("/settings", api.updateSettings)
			r.Get("/app/update", api.appUpdate)
			r.Post("/app/update", api.selfUpdate)
			r.Get("/updates", api.listUpdates)
			r.Post("/updates/check", api.checkUpdates)
			r.Put("/updates/ignore", api.setIgnore)
			r.Get("/home", api.listHome)
			r.Get("/home/layout", api.getHomeLayout)
			r.Put("/home/layout", api.putHomeLayout)
			r.Put("/home/{stack}/{service}/visibility", api.setVisibility)
			r.Put("/home/{stack}/{service}/icon", api.setIcon)
			r.Put("/home/{stack}/{service}/name", api.setName)
			r.Put("/home/{stack}/{service}/url", api.setUrl)
			r.Get("/icons/{slug}", api.icon)
		})
	})

	// Everything else is the SPA (client-side routing → index.html fallback).
	r.NotFound(SPAHandler(dist, logger))

	api.mux = r
	return api
}

type api struct {
	cfg     config.Config
	logger  *slog.Logger
	db      *store.Store
	stacks  *stacks.Manager
	hub     *events.Hub
	host    *hoststats.Sampler
	docker  *docker.Client // may be nil (no daemon)
	icons   *discovery.IconResolver
	runner  *compose.Runner
	checker *updates.Checker
	mux     http.Handler   // the built router (returned by New)
	login   *loginLimiter  // brute-force damper for login/setup

	checking     atomic.Bool // guards against concurrent update-check runs
	selfUpdating atomic.Bool // guards against concurrent self-updates
	selfCheck    selfCheckCache

	setupMu    sync.Mutex // guards setupToken
	setupToken string     // one-time first-run token ("" once setup completes / when bootstrapped)
}

func (a *api) hostStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.host.Snapshot())
}

type healthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	StacksDir string `json:"stacksDir"`
	Time      string `json:"time"`
}

func (a *api) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:    "ok",
		Version:   version,
		StacksDir: a.cfg.StacksDir,
		Time:      time.Now().UTC().Format(time.RFC3339),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Response already partially written; nothing safe left to do but log.
		slog.Error("encode json response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// peerCtxKey holds the real TCP peer IP captured before any proxy-header
// rewriting. Unexported type prevents collisions with other context values.
type peerCtxKey struct{}

// capturePeer stashes the request's genuine TCP peer IP in the context before
// middleware.RealIP can rewrite RemoteAddr from X-Forwarded-For. Trusted-header
// auth reads this, never the (spoofable) rewritten address.
func capturePeer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ctx := context.WithValue(r.Context(), peerCtxKey{}, host)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// peerIP returns the real TCP peer IP captured by capturePeer, or "" if absent.
func peerIP(r *http.Request) string {
	if v, ok := r.Context().Value(peerCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// clientIP returns the best-guess client IP for rate-limiting and audit logs —
// r.RemoteAddr after middleware.RealIP has applied X-Forwarded-For (so it's the
// real client behind a proxy). Unlike peerIP this is advisory, not a trust
// boundary; it only keys a brute-force damper.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// securityHeaders sets baseline browser protections on every response. The CSP
// is intentionally permissive enough for the embedded SPA (inline styles via
// Tailwind's generated CSS are fine; no inline scripts are used) while blocking
// framing, MIME sniffing, and cross-origin embedding of the UI.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		// img-src allows https + data for user-set custom icon URLs; connect-src
		// 'self' covers the API + WebSocket (ws: is same-origin via 'self').
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' https: data:; style-src 'self' 'unsafe-inline'; "+
				"script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
		next.ServeHTTP(w, r)
	})
}

// requestLogger is a small slog-backed access logger (chi's default logger
// writes to the standard logger, not slog).
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			logger.Debug("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"dur_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}
