package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/rogalinski/hivedock/internal/hostops"
)

// updateService rewrites one service's image tag in a managed stack's compose
// file (comment-preserving, byte-exact) and saves it. Two phases (§5.2): a
// request without confirm returns a unified diff of exactly what would change
// (no write); a request with confirm:true applies it, but only if the file still
// matches baseSha256 from the preview (optimistic lock, §5.1) — so a machine
// edit never silently clobbers a concurrent change and the user sees the diff
// first. Save ≠ deploy: the caller redeploys separately. Env-interpolated and
// digest-pinned images are surfaced (409), never rewritten.
func (a *api) updateService(w http.ResponseWriter, r *http.Request) {
	be, err := a.backendFor(hostParam(r))
	if err != nil {
		a.httpError(w, err)
		return
	}
	name := chi.URLParam(r, "name")
	service := chi.URLParam(r, "service")

	var body struct {
		Tag        string `json:"tag"`
		Confirm    bool   `json:"confirm"`
		BaseSha256 string `json:"baseSha256"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body.Tag = strings.TrimSpace(body.Tag)
	if body.Tag == "" {
		writeError(w, http.StatusBadRequest, "tag is required")
		return
	}

	res, err := be.UpdateService(r.Context(), hostops.UpdateServiceReq{
		Name: name, Service: service, Tag: body.Tag, Confirm: body.Confirm, BaseSha256: body.BaseSha256,
	})
	if err != nil {
		a.httpError(w, err)
		return
	}
	// Notify only on an actual applied change (not a preview or a no-op).
	if res.Changed && !res.Preview {
		a.logger.Info("service image updated", "host", hostParam(r), "stack", name, "service", service, "tag", body.Tag)
		a.hub.NotifyChanged("update:" + name)
	}
	writeJSON(w, http.StatusOK, res)
}
