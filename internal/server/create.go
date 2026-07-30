package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

// createStack scaffolds a new managed stack: a directory under the host's
// STACKS_DIR plus a template compose.yaml (or a supplied compose body). It does
// not deploy — the user edits then deploys. Auth + CSRF protected (mutating POST
// in the guarded group).
func (a *api) createStack(w http.ResponseWriter, r *http.Request) {
	be, err := a.backendFor(hostParam(r))
	if err != nil {
		a.httpError(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxComposeBytes)
	var body struct {
		Name string `json:"name"`
		// Compose, when set, is written as the new stack's compose.yaml instead of
		// the nginx starter (e.g. the output of the "docker run → compose"
		// converter). It's the user's own compose text — validated on the Compose
		// tab and at deploy, same as any edit — so it's written as-is.
		Compose string `json:"compose,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if _, isMax := err.(*http.MaxBytesError); isMax {
			writeError(w, http.StatusRequestEntityTooLarge, "compose file too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ref, err := be.CreateStack(r.Context(), strings.TrimSpace(body.Name), body.Compose)
	if err != nil {
		a.httpError(w, err)
		return
	}
	a.logger.Info("stack created", "host", hostParam(r), "name", ref.Name, "dir", ref.Dir)
	a.hub.NotifyChanged("stack:created")
	writeJSON(w, http.StatusCreated, ref)
}
