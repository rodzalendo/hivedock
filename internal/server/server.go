// Package server wires the HTTP router: REST for request/response, a single
// multiplexed WebSocket for streams, and the embedded SPA as the fallback.
package server

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/rogalinski/hivedock/internal/config"
	"github.com/rogalinski/hivedock/internal/events"
	"github.com/rogalinski/hivedock/internal/stacks"
	"github.com/rogalinski/hivedock/internal/store"
)

// version is the build-time version string; overridden via -ldflags in the
// Dockerfile. "dev" for local builds.
var version = "dev"

// New builds the top-level HTTP handler.
func New(cfg config.Config, logger *slog.Logger, db *store.Store, stacksSvc *stacks.Manager, hub *events.Hub, dist fs.FS) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)

	api := &api{cfg: cfg, logger: logger, db: db, stacks: stacksSvc, hub: hub}

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", api.health)
		r.Get("/ws", api.websocket)
		r.Get("/stacks", api.listStacks)
		r.Get("/stacks/{name}", api.getStack)
	})

	// Everything else is the SPA (client-side routing → index.html fallback).
	r.NotFound(SPAHandler(dist, logger))

	return r
}

type api struct {
	cfg    config.Config
	logger *slog.Logger
	db     *store.Store
	stacks *stacks.Manager
	hub    *events.Hub
}

type healthResponse struct {
	Status       string `json:"status"`
	Version      string `json:"version"`
	StacksDir    string `json:"stacksDir"`
	AuthDisabled bool   `json:"authDisabled"`
	Time         string `json:"time"`
}

func (a *api) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:       "ok",
		Version:      version,
		StacksDir:    a.cfg.StacksDir,
		AuthDisabled: a.cfg.AuthDisabled,
		Time:         time.Now().UTC().Format(time.RFC3339),
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
