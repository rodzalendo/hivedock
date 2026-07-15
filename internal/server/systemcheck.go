package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
)

// runSystemChecks runs the boot-time environment checks (§6.3, §6.4) once, before
// the server starts serving. Results are stored on the api and never mutated
// after, so requests read them without a lock. No daemon (dev) → nothing to check.
func (a *api) runSystemChecks(ctx context.Context) {
	if a.docker == nil {
		return
	}

	// §6.4 — Podman / rootless Docker are unsupported; say so explicitly rather
	// than break in confusing ways later.
	if rootless, podman := a.docker.DaemonRuntime(ctx); podman || rootless {
		if podman {
			a.systemWarnings = append(a.systemWarnings,
				"Podman detected. HiveDock targets Docker Engine and is unsupported on Podman — the socket API and compose behavior differ.")
		}
		if rootless {
			a.systemWarnings = append(a.systemWarnings,
				"Rootless Docker detected. HiveDock is unsupported on rootless Docker — socket path, permissions, and bind semantics differ.")
		}
	}

	// §6.3 — Invariant 4: STACKS_DIR must resolve to the same path inside and
	// outside the container, or compose relative-path resolution silently points
	// at the wrong files. Verify HiveDock's own bind and refuse to mutate on a
	// mismatch instead of corrupting stacks.
	hostname, _ := os.Hostname() // inside a container this is the container id
	if hostname == "" {
		return
	}
	src, found, err := a.docker.SelfBindSource(ctx, hostname, a.cfg.StacksDir)
	switch {
	case err != nil:
		// Not running as an inspectable container (dev, plain binary): can't
		// verify, don't assume broken.
		a.logger.Debug("startup self-check: could not inspect own container", "err", err)
	case found && src != a.cfg.StacksDir:
		a.readOnlyReason = fmt.Sprintf(
			"STACKS_DIR bind mismatch: the host path %q is mounted at %q. Docker Compose resolves relative paths against the host path, so a mismatch points them at the wrong files. Fix the bind to %q:%q and restart. HiveDock is running read-only until then.",
			src, a.cfg.StacksDir, a.cfg.StacksDir, a.cfg.StacksDir)
		a.systemWarnings = append(a.systemWarnings, a.readOnlyReason)
		a.logger.Error("startup self-check FAILED: STACKS_DIR bind mismatch — entering read-only mode",
			"host_source", src, "container_dest", a.cfg.StacksDir)
	case !found:
		a.logger.Debug("startup self-check: STACKS_DIR is not a bind mount; parity not verified")
	default:
		a.logger.Info("startup self-check: STACKS_DIR bind parity OK", "path", a.cfg.StacksDir)
	}
}

// enforceReadOnly refuses unsafe methods with 503 when a boot check put HiveDock
// in read-only mode (§6.3). Reads still work, so the UI stays usable and can
// surface the banner explaining what to fix.
func (a *api) enforceReadOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.readOnlyReason != "" {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				writeError(w, http.StatusServiceUnavailable, a.readOnlyReason)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
