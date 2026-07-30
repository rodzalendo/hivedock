package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// deleteStack removes a managed stack: it tears down its containers first (so
// nothing is orphaned), then deletes the stack's directory under STACKS_DIR.
// With ?volumes=true it also deletes the stack's named volumes. External stacks
// are read-only and can't be deleted. Auth + CSRF protected. This is
// destructive — the compose file and everything in the stack's directory is
// removed.
func (a *api) deleteStack(w http.ResponseWriter, r *http.Request) {
	host := hostParam(r)
	name := chi.URLParam(r, "name")
	withVolumes := r.URL.Query().Get("volumes") == "true"

	be, err := a.backendFor(host)
	if err != nil {
		a.httpError(w, err)
		return
	}

	// Serialize with any in-flight deploy for this stack (same named lock).
	release, acquired := a.runner.Start(lockKey(host, name))
	if !acquired {
		writeError(w, http.StatusConflict, "an operation is already running for this stack")
		return
	}
	defer release()

	if err := be.DeleteStack(r.Context(), name, withVolumes); err != nil {
		a.httpError(w, err)
		return
	}
	a.logger.Info("stack deleted", "host", host, "name", name)
	a.hub.NotifyChanged("stack:deleted")
	w.WriteHeader(http.StatusNoContent)
}

// renameStack renames a managed stack's directory (its identity + compose
// project name). The stack must be stopped first: renaming a running stack
// would change its compose project name and orphan the live containers, so a
// running stack is rejected with a 409.
func (a *api) renameStack(w http.ResponseWriter, r *http.Request) {
	host := hostParam(r)
	name := chi.URLParam(r, "name")
	var body struct {
		NewName string `json:"newName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	newName := strings.TrimSpace(body.NewName)
	if newName == name {
		writeError(w, http.StatusBadRequest, "new name is the same as the current name")
		return
	}

	be, err := a.backendFor(host)
	if err != nil {
		a.httpError(w, err)
		return
	}
	ref, err := be.RenameStack(r.Context(), name, newName)
	if err != nil {
		a.httpError(w, err)
		return
	}

	// Carry the visibility/icon prefs over to the new name. Prefs live in the
	// manager's DB and are keyed by stack name only (Home is local-scoped), so
	// this applies to the local host.
	if host == "local" && a.db != nil {
		if err := a.db.RenameStackPrefs(name, newName); err != nil {
			a.logger.Warn("rename stack: move prefs", "from", name, "to", newName, "err", err)
		}
	}

	a.logger.Info("stack renamed", "host", host, "from", name, "to", newName, "dir", ref.Dir)
	a.hub.NotifyChanged("stack:renamed")
	writeJSON(w, http.StatusOK, ref)
}
