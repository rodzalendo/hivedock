package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// getEnv returns a managed stack's .env file. A stack often has none yet, so a
// missing file is a normal 200 with empty content and exists:false (not a 404) —
// the editor opens blank and a save creates it.
func (a *api) getEnv(w http.ResponseWriter, r *http.Request) {
	be, err := a.backendFor(hostParam(r))
	if err != nil {
		a.httpError(w, err)
		return
	}
	ef, err := be.GetEnv(r.Context(), chi.URLParam(r, "name"))
	if err != nil {
		a.httpError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ef)
}

// putEnv atomically writes a managed stack's .env file, creating it if needed.
// Like the compose editor: save ≠ deploy — the change applies on the next
// deploy (and shows as drift until then). No `docker compose config` validation
// is needed; .env is plain KEY=VALUE that compose interpolates at runtime.
func (a *api) putEnv(w http.ResponseWriter, r *http.Request) {
	be, err := a.backendFor(hostParam(r))
	if err != nil {
		a.httpError(w, err)
		return
	}
	content, baseSha, ok := readContentBody(w, r)
	if !ok {
		return
	}
	name := chi.URLParam(r, "name")
	ef, err := be.PutEnv(r.Context(), name, content, baseSha)
	if err != nil {
		a.httpError(w, err)
		return
	}
	a.logger.Info("env saved", "host", hostParam(r), "stack", name, "bytes", len(content))
	a.hub.NotifyChanged("env:saved")
	writeJSON(w, http.StatusOK, ef)
}
