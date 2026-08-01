package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/rogalinski/hivedock/internal/compose"
	"github.com/rogalinski/hivedock/internal/events"
	"github.com/rogalinski/hivedock/internal/hostops"
)

// runStackAction triggers a mutating compose operation (up/down/restart/pull/
// stop) on a managed stack — local or remote. The mutation is triggered here —
// over an authenticated, CSRF-protected POST — and its output is streamed back
// over the WebSocket as deploy:* messages, tagged with the host. The operation
// runs on a background context so a browser refresh (or WS drop) can't abort an
// in-flight deploy; the daemon on the owning host holds the containers regardless
// of this process's lifetime.
func (a *api) runStackAction(w http.ResponseWriter, r *http.Request) {
	host := hostParam(r)
	name := chi.URLParam(r, "name")
	action := compose.Action(chi.URLParam(r, "action"))
	if !action.Valid() {
		writeError(w, http.StatusBadRequest, "unknown action: "+string(action))
		return
	}

	be, err := a.backendFor(host)
	if err != nil {
		a.httpError(w, err)
		return
	}
	// Reject a missing/external stack synchronously (before accepting the deploy).
	if _, err := a.managedStack(r.Context(), be, name); err != nil {
		a.httpError(w, err)
		return
	}

	release, acquired := a.runner.Start(lockKey(host, name))
	if !acquired {
		writeError(w, http.StatusConflict, "an operation is already running for this stack")
		return
	}

	opID := newOpID()
	go a.executeDeploy(host, be, name, string(action), "", opID, release)

	writeJSON(w, http.StatusAccepted, map[string]string{
		"id": opID, "host": host, "stack": name, "action": string(action),
	})
}

// restartService restarts a single service of a managed stack — the smallest
// useful hammer when one container misbehaves. Output streams over the
// WebSocket exactly like a stack-level action, under the same per-stack lock.
func (a *api) restartService(w http.ResponseWriter, r *http.Request) {
	host := hostParam(r)
	name := chi.URLParam(r, "name")
	service := chi.URLParam(r, "service")

	be, err := a.backendFor(host)
	if err != nil {
		a.httpError(w, err)
		return
	}
	st, err := a.managedStack(r.Context(), be, name)
	if err != nil {
		a.httpError(w, err)
		return
	}
	found := false
	for _, svc := range st.Services {
		if svc.Name == service {
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "service not found in stack: "+service)
		return
	}

	release, acquired := a.runner.Start(lockKey(host, name))
	if !acquired {
		writeError(w, http.StatusConflict, "an operation is already running for this stack")
		return
	}

	opID := newOpID()
	go a.executeDeploy(host, be, name, string(compose.ActionRestart), service, opID, release)

	writeJSON(w, http.StatusAccepted, map[string]string{
		"id": opID, "host": host, "stack": name, "action": string(compose.ActionRestart), "service": service,
	})
}

// executeDeploy runs the operation via the host's backend and broadcasts
// start/line/end over the hub, tagged with host so the browser can attribute
// output to the right host's stack. Runs on a manager-lifetime context.
func (a *api) executeDeploy(host string, be hostops.Backend, name, action, service, opID string, release func()) {
	defer release()

	a.hub.Publish(events.Message{Type: "deploy:start", Payload: map[string]string{
		"id": opID, "host": host, "stack": name, "action": action, "service": service,
	}})
	a.logger.Info("deploy start", "host", host, "stack", name, "action", action, "id", opID)

	err := be.RunAction(context.Background(), name, action, service, func(line string) {
		a.hub.Publish(events.Message{Type: "deploy:line", Payload: map[string]string{
			"id": opID, "host": host, "stack": name, "line": line,
		}})
	})

	end := map[string]any{"id": opID, "host": host, "stack": name, "action": action, "service": service, "ok": err == nil}
	if err != nil {
		end["error"] = err.Error()
		a.logger.Warn("deploy failed", "host", host, "stack", name, "action", action, "id", opID, "err", err)
	} else {
		a.logger.Info("deploy ok", "host", host, "stack", name, "action", action, "id", opID)
	}
	a.hub.Publish(events.Message{Type: "deploy:end", Payload: end})

	// The operation changed container state; nudge clients to refetch the truth
	// model (docker events usually cover this, but not for pull/no-op cases).
	a.hub.NotifyChanged("deploy:" + action)

	// A successful pull/redeploy invalidates this stack's cached update results.
	// Refresh them here so the Updates page is correct even if the browser never
	// asks for a re-check (or its request lost the race with a scheduled sweep).
	// Local host only: the update cache is keyed by image across the manager's own
	// stacks, and the checker inspects the manager's daemon for local digests.
	if err == nil && host == "local" && movesImages(action) {
		go a.recheckStackImages(name)
	}
}

// movesImages reports whether an action can change which image a service runs,
// and therefore whether the cached update check for it is now stale.
func movesImages(action string) bool {
	switch compose.Action(action) {
	case compose.ActionUpdate, compose.ActionPull, compose.ActionUp, compose.ActionRecreate:
		return true
	default:
		return false
	}
}

// newOpID returns a short random hex id used to correlate deploy:* messages.
func newOpID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "op"
	}
	return hex.EncodeToString(b)
}
