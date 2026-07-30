package server

import (
	"context"
	"net/http"

	"github.com/rogalinski/hivedock/internal/hostops"
)

// runSystemChecks runs the boot-time environment checks (§6.3, §6.4) once, before
// the server starts serving. Results are stored on the api and never mutated
// after, so requests read them without a lock. The check itself lives in hostops
// so a remote agent runs the identical bind-parity guard (docs/MULTIHOST.md).
func (a *api) runSystemChecks(ctx context.Context) {
	a.systemWarnings, a.readOnlyReason = hostops.SystemCheck(ctx, a.docker, a.cfg.StacksDir, a.logger)
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
