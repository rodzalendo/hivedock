package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/rogalinski/hivedock/internal/hostops"
	"github.com/rogalinski/hivedock/internal/stacks"
)

// The stack-management handlers are host-agnostic: each resolves a hostops.Backend
// for the request's host and calls it, so local and remote stacks go through
// identical code (docs/MULTIHOST.md). "local" is the manager's own host (an
// in-process LocalBackend); any other name is a connected agent, reached over the
// RPC by a remoteBackend. The unscoped /api/stacks/… routes carry no {host} param
// and default to "local", so single-host behavior is unchanged.

// The wire DTOs are the portable hostops result types — the browser receives the
// same JSON whether a stack is local or remote. Aliases keep the old names (and
// the tests that use them) pointing at the shared structs.
type (
	composeFileResponse   = hostops.ComposeFile
	envFileResponse       = hostops.EnvFile
	createStackResponse   = hostops.StackRef
	updateServiceResponse = hostops.UpdateServiceResult
)

// sha256hex is the lowercase hex SHA-256 of b (kept as a package-local alias for
// the read-only-token hashing in apitoken.go).
func sha256hex(b []byte) string { return hostops.Sha256Hex(b) }

// hostParam returns the {host} route segment, defaulting to "local" for the
// unscoped /api/stacks/… routes (which carry no host param).
func hostParam(r *http.Request) string {
	if h := chi.URLParam(r, "host"); h != "" {
		return h
	}
	return "local"
}

// backendFor resolves the stack-management backend for a host: the in-process
// LocalBackend for "local", else the connected agent's remoteBackend. A named but
// unconnected host is reported offline (→ 502).
func (a *api) backendFor(host string) (hostops.Backend, error) {
	if host == "" || host == "local" {
		return a.local, nil
	}
	h := a.hosts.get(host)
	if h == nil {
		return nil, hostops.ErrOffline
	}
	return newRemoteBackend(h), nil
}

// lockKey namespaces the per-stack operation lock by host, so a same-named stack
// on two hosts never shares a lock (the manager's runner mutex is reused as a
// plain named lock for both local and remote ops).
func lockKey(host, stack string) string { return host + "/" + stack }

// managedStack loads a stack and requires it to be managed (has a compose file
// under STACKS_DIR); external stacks are read-only. Returns typed errors the
// caller maps with httpError.
func (a *api) managedStack(ctx context.Context, be hostops.Backend, name string) (stacks.Stack, error) {
	st, err := be.GetStack(ctx, name)
	if err != nil {
		return st, err
	}
	if st.Origin != stacks.OriginManaged || st.ComposeFile == "" {
		return st, hostops.ErrExternal
	}
	return st, nil
}

// httpError maps a hostops domain error to the right HTTP status and body. A
// conflict keeps the current bytes so the editor's 409-reconcile flow works the
// same for a remote stack as for a local one.
func (a *api) httpError(w http.ResponseWriter, err error) {
	var conflict *hostops.ConflictError
	if errors.As(err, &conflict) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   conflict.Error(),
			"content": conflict.Content,
			"sha256":  conflict.Sha256,
		})
		return
	}
	writeError(w, statusForCode(hostops.CodeFor(err)), err.Error())
}

// statusForCode maps a machine error code to an HTTP status. An empty/unknown
// code is a generic 500.
func statusForCode(code string) int {
	switch code {
	case hostops.CodeNotFound, hostops.CodeServiceNotFound:
		return http.StatusNotFound
	case hostops.CodeExternal, hostops.CodeBusy, hostops.CodeExists, hostops.CodeRunning,
		hostops.CodeConflict, hostops.CodeEnvManaged, hostops.CodeDigestPinned, hostops.CodeNoImage:
		return http.StatusConflict
	case hostops.CodeValidation:
		return http.StatusUnprocessableEntity
	case hostops.CodeInvalidName, hostops.CodeEscape:
		return http.StatusBadRequest
	case hostops.CodeReadOnly:
		return http.StatusForbidden
	case hostops.CodeNoDocker:
		return http.StatusServiceUnavailable
	case hostops.CodeOffline:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}
