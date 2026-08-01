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
	"strings"
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
	"github.com/rogalinski/hivedock/internal/hostops"
	"github.com/rogalinski/hivedock/internal/hoststats"
	"github.com/rogalinski/hivedock/internal/registry"
	"github.com/rogalinski/hivedock/internal/stacks"
	"github.com/rogalinski/hivedock/internal/store"
	"github.com/rogalinski/hivedock/internal/updates"
)

// version is the build-time version string; overridden via -ldflags in the
// Dockerfile. "dev" for local builds.
var version = "dev"

// Version returns the build-time version string (used by the `hivedock agent`
// CLI, which reports it to the manager on connect).
func Version() string { return version }

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
	// capturePeer records the genuine TCP peer for trusted-header auth. There is
	// deliberately no chi middleware.RealIP here: it rewrites RemoteAddr from
	// attacker-controlled headers for every request, whether or not a proxy set
	// them (chi deprecated it for exactly that — GHSA-3fxj-6jh8-hvhx). Handlers
	// that want the client address call a.clientIP, which only believes
	// X-Forwarded-For when the peer is a configured trusted proxy.
	r.Use(capturePeer)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)

	// The update checker uses the Docker client (nil-safe) for local image
	// digests + changelog source labels on the mutable-tag path.
	var local updates.LocalImages
	if dockerClient != nil {
		local = dockerClient
	}
	// One registry client, shared by the update checker and the self-update
	// digest resolver, wired to per-registry credentials + TLS trust from the
	// store (§6.1/§6.2). The resolver reads the store per call, so edits apply
	// without a restart.
	regClient := registry.NewClient(nil)
	if db != nil {
		regClient.SetConfigResolver(registryConfigResolver(db))
	}
	checker := updates.NewChecker(regClient, local, logger)

	api := &api{cfg: cfg, logger: logger, db: db, stacks: stacksSvc, hub: hub, host: host, docker: dockerClient, icons: icons, runner: compose.NewRunner(), checker: checker, login: newLoginLimiter(),
		verify:  cosignVerifier{},  // §3.2 in-app signature verification (bundled cosign)
		selfReg: regClient,         // resolves the candidate release digest to verify
		hosts:   newHostRegistry(), // multi-host: connected remote agents (docs/MULTIHOST.md)
	}
	// The local host's stack-management backend: the portable hostops core over
	// this manager's STACKS_DIR + docker socket + git auto-commit. Remote hosts get
	// a remoteBackend over the agent RPC; both satisfy hostops.Backend so the stack
	// handlers are host-agnostic (docs/MULTIHOST.md).
	api.local = hostops.NewLocal(cfg.StacksDir, stacksSvc, api.runner, dockerClient, api.gitAutoCommitEnabled, logger)

	// First-run: bootstrap the admin from env, or mint a one-time setup token.
	api.initFirstRun()

	// Boot-time environment checks: runtime detection + STACKS_DIR bind parity
	// (§6.3/§6.4). May put the server in read-only mode.
	api.runSystemChecks(ctx)

	// Periodic background update checks (settings override CHECK_INTERVAL;
	// cadence changes apply live).
	api.startUpdateScheduler(ctx)

	r.Route("/api", func(r chi.Router) {
		// Public: liveness + the auth bootstrap (status/setup/login).
		r.Get("/health", api.health)
		// Remote agents enroll here (token-gated, not session-gated —
		// docs/MULTIHOST.md); disabled unless AGENT_TOKEN is set.
		r.Get("/agent/connect", api.agentConnect)
		r.Route("/auth", func(r chi.Router) {
			r.Get("/status", api.authStatus)
			r.Post("/setup", api.authSetup)
			r.Post("/login", api.authLogin)
			// Logout is deliberately NOT behind requireAuth: it must stay
			// reachable even when the CSRF cookie is missing, so a stale-cookie
			// state can always be cleared from inside the app instead of becoming
			// a hard lockout (it and every other mutation would 403). The handler
			// only ever clears the caller's own cookies and deletes the session
			// named by the caller's own cookie; forcing a logout is low-harm
			// (worst case: a return to the login screen).
			r.Post("/logout", api.authLogout)
		})

		// Everything else requires a session or a trusted-proxy header. When a
		// boot check put the server in read-only mode, unsafe methods are refused.
		r.Group(func(r chi.Router) {
			r.Use(api.requireAuth)
			r.Use(api.enforceReadOnly)
			r.Get("/ws", api.websocket)
			r.Get("/containers/{id}/exec", api.containerExec)
			r.Get("/hosts", api.listHosts)
			// The stack-management surface. The same handlers serve the unscoped
			// /stacks/… routes (implicit host "local") and the host-scoped mirror
			// /hosts/{host}/stacks/… — each handler reads hostParam(r) and resolves
			// a backend, so local and remote go through identical code
			// (docs/MULTIHOST.md).
			mountStacks := func(r chi.Router) {
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
			}
			r.Route("/hosts/{host}", func(r chi.Router) {
				r.Get("/containers", api.hostContainers)
				mountStacks(r)
			})
			mountStacks(r)
			r.Get("/host/stats", api.hostStats)
			r.Post("/system/prune", api.prune)
			r.Get("/settings", api.settings)
			r.Put("/settings", api.updateSettings)
			r.Post("/settings/git-init", api.gitInit)
			r.Post("/settings/git-pull", api.gitPull)
			r.Post("/settings/api-token", api.generateAPIToken)
			r.Delete("/settings/api-token", api.revokeAPIToken)
			r.Post("/settings/agent-token", api.generateAgentToken)
			r.Delete("/settings/agent-token", api.revokeAgentToken)
			r.Get("/settings/registries", api.listRegistries)
			r.Put("/settings/registries", api.putRegistry)
			r.Delete("/settings/registries", api.deleteRegistry)
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
			r.Get("/icons/remote", api.remoteIcon)
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
	verify  imageVerifier         // §3.2 cosign signature verification (execs bundled cosign)
	selfReg selfRegistry          // resolves the candidate release digest to verify/deploy
	mux     http.Handler          // the built router (returned by New)
	login   *loginLimiter         // brute-force damper for login/setup
	hosts   *hostRegistry         // multi-host: connected remote agents (docs/MULTIHOST.md)
	local   *hostops.LocalBackend // the local host's stack-management backend (docs/MULTIHOST.md)

	checking     atomic.Bool // guards against concurrent update-check runs
	selfUpdating atomic.Bool // guards against concurrent self-updates
	selfCheck    selfCheckCache

	setupMu    sync.Mutex // guards setupToken
	setupToken string     // one-time first-run token ("" once setup completes / when bootstrapped)

	// Boot-time system-check results (§6.3/§6.4), set once before serving and
	// read without a lock thereafter.
	systemWarnings []string // podman/rootless/bind-mismatch notices for the UI banner
	readOnlyReason string   // non-empty → mutations refused (STACKS_DIR bind mismatch)
}

func (a *api) hostStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.host.Snapshot())
}

type healthResponse struct {
	Status    string   `json:"status"`
	Version   string   `json:"version"`
	StacksDir string   `json:"stacksDir"`
	Time      string   `json:"time"`
	ReadOnly  bool     `json:"readOnly,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

func (a *api) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:    "ok",
		Version:   version,
		StacksDir: a.cfg.StacksDir,
		Time:      time.Now().UTC().Format(time.RFC3339),
		ReadOnly:  a.readOnlyReason != "",
		Warnings:  a.systemWarnings,
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

// capturePeer stashes the request's genuine TCP peer IP in the context, so
// trusted-header auth always has an address no header can influence, whatever
// else later touches r.RemoteAddr.
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

// clientIP returns the client address used to key the login brute-force damper.
//
// X-Forwarded-For is honored ONLY when the direct peer is a configured trusted
// proxy; otherwise the peer address wins. That distinction is the whole point:
// trusting the header unconditionally (what chi's deprecated middleware.RealIP
// did) lets anyone reaching the server directly mint a fresh rate-limit bucket
// per request by varying the header, which defeats the damper it feeds.
//
// Still not a trust boundary — authentication decisions use peerIP.
func (a *api) clientIP(r *http.Request) string {
	peer := peerIP(r)
	if peer == "" {
		peer = hostOnly(r.RemoteAddr)
	}
	if !a.peerTrusted(peer) {
		return peer
	}
	// Leftmost XFF entry is the originating client as recorded by our own proxy.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		return real
	}
	return peer
}

// hostOnly strips the port from a host:port address, tolerating a bare host.
func hostOnly(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
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
		// Zero external origins (§4.5): the SPA is embedded and every icon —
		// including user-set custom URLs — is proxied and served from 'self', so
		// img-src needs only 'self' + data:. style-src keeps 'unsafe-inline' for
		// Tailwind's generated CSS; no inline scripts are used. connect-src 'self'
		// covers the API + same-origin WebSocket.
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; "+
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
