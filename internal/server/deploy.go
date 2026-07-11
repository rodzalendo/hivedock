package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/rogalinski/hivedock/internal/compose"
	"github.com/rogalinski/hivedock/internal/events"
	"github.com/rogalinski/hivedock/internal/stacks"
)

// runStackAction triggers a mutating compose operation (up/down/restart/pull/
// stop) on a managed stack. The mutation is triggered here — over an
// authenticated, CSRF-protected POST — and its output is streamed back over the
// WebSocket as deploy:* messages. The operation runs on a background context so
// a browser refresh (or WS drop) can't abort an in-flight deploy; the docker
// daemon owns the containers regardless of this process's lifetime.
func (a *api) runStackAction(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	action := compose.Action(chi.URLParam(r, "action"))
	if !action.Valid() {
		writeError(w, http.StatusBadRequest, "unknown action: "+string(action))
		return
	}

	st, ok, err := a.stacks.Get(r.Context(), name)
	if err != nil {
		a.logger.Error("deploy: get stack", "name", name, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to load stack: "+err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "stack not found: "+name)
		return
	}
	// Only managed stacks (with a compose file under STACKS_DIR) are mutable;
	// external stacks are read-only — the UI never lies about ownership.
	if st.Origin != stacks.OriginManaged || st.ComposeFile == "" {
		writeError(w, http.StatusConflict, "stack is external (read-only); no compose file to operate on")
		return
	}

	// Acquire the per-stack lock synchronously so we can 409 the caller if an
	// operation is already running for this stack.
	release, acquired := a.runner.Start(name)
	if !acquired {
		writeError(w, http.StatusConflict, "an operation is already running for this stack")
		return
	}

	opID := newOpID()
	op := compose.Op{
		Stack:       name,
		Action:      action,
		ComposeFile: st.ComposeFile,
		ProjectDir:  st.Dir,
	}

	go a.executeDeploy(op, opID, release)

	writeJSON(w, http.StatusAccepted, map[string]string{
		"id":     opID,
		"stack":  name,
		"action": string(action),
	})
}

// executeDeploy runs the operation and broadcasts start/line/end over the hub.
func (a *api) executeDeploy(op compose.Op, opID string, release func()) {
	defer release()

	a.hub.Publish(events.Message{Type: "deploy:start", Payload: map[string]string{
		"id": opID, "stack": op.Stack, "action": string(op.Action),
	}})
	a.logger.Info("deploy start", "stack", op.Stack, "action", op.Action, "id", opID)

	err := a.runner.Exec(context.Background(), op, func(line string) {
		a.hub.Publish(events.Message{Type: "deploy:line", Payload: map[string]string{
			"id": opID, "stack": op.Stack, "line": line,
		}})
	})

	end := map[string]any{"id": opID, "stack": op.Stack, "action": string(op.Action), "ok": err == nil}
	if err != nil {
		end["error"] = err.Error()
		a.logger.Warn("deploy failed", "stack", op.Stack, "action", op.Action, "id", opID, "err", err)
	} else {
		a.logger.Info("deploy ok", "stack", op.Stack, "action", op.Action, "id", opID)
	}
	a.hub.Publish(events.Message{Type: "deploy:end", Payload: end})

	// The operation changed container state; nudge clients to refetch the truth
	// model (docker events usually cover this, but not for pull/no-op cases).
	a.hub.NotifyChanged("deploy:" + string(op.Action))
}

// newOpID returns a short random hex id used to correlate deploy:* messages.
func newOpID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "op"
	}
	return hex.EncodeToString(b)
}
