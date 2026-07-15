package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/rogalinski/hivedock/internal/compose"
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
	st, ok := a.managedComposeFile(w, r)
	if !ok {
		return
	}
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

	real, ok := a.containedPath(w, st.ComposeFile)
	if !ok {
		return
	}
	content, err := os.ReadFile(real)
	if err != nil {
		a.logger.Error("update service: read compose", "path", st.ComposeFile, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to read compose file: "+err.Error())
		return
	}

	updated, err := compose.SetImageTag(content, service, body.Tag)
	switch {
	case errors.Is(err, compose.ErrEnvManaged):
		writeError(w, http.StatusConflict, "image tag is managed via .env; edit the .env file instead")
		return
	case errors.Is(err, compose.ErrDigestPinned):
		writeError(w, http.StatusConflict, "image is pinned by digest; not rewriting")
		return
	case errors.Is(err, compose.ErrServiceNotFound):
		writeError(w, http.StatusNotFound, "service not found: "+service)
		return
	case errors.Is(err, compose.ErrNoImage):
		writeError(w, http.StatusConflict, "service has no image to update: "+service)
		return
	case err != nil:
		a.logger.Error("update service: rewrite", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to update image tag: "+err.Error())
		return
	}

	label := st.Name + "/" + filepath.Base(st.ComposeFile)
	if bytes.Equal(updated, content) {
		writeJSON(w, http.StatusOK, updateServiceResponse{
			Stack: st.Name, Service: service, Tag: body.Tag, Changed: false, Sha256: sha256hex(content),
		})
		return
	}

	if !body.Confirm {
		// Preview: show exactly what would change, and hand back the base hash so
		// the apply that follows is locked to this file state.
		writeJSON(w, http.StatusOK, updateServiceResponse{
			Stack: st.Name, Service: service, Tag: body.Tag, Changed: true,
			Preview: true, Diff: unifiedDiff(content, updated, label), Sha256: sha256hex(content),
		})
		return
	}

	// Apply: refuse if the file moved under us since the preview.
	if !a.checkOptimisticLock(w, real, body.BaseSha256) {
		return
	}
	gitAction := "update " + st.Name + "/" + service + " to " + body.Tag
	if !a.gitSnapshotBefore(w, gitAction) {
		return
	}
	if err := atomicWrite(real, updated); err != nil {
		a.logger.Error("update service: write compose", "path", st.ComposeFile, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save compose file: "+err.Error())
		return
	}
	if err := a.gitCommitAfter(gitAction); err != nil {
		a.logger.Error("update service: git commit", "err", err)
		writeError(w, http.StatusInternalServerError, "updated, but the git commit failed: "+err.Error())
		return
	}
	a.logger.Info("service image updated", "stack", st.Name, "service", service, "tag", body.Tag)
	a.hub.NotifyChanged("update:" + st.Name)
	writeJSON(w, http.StatusOK, updateServiceResponse{
		Stack: st.Name, Service: service, Tag: body.Tag, Changed: true, Sha256: sha256hex(updated),
	})
}

type updateServiceResponse struct {
	Stack   string `json:"stack"`
	Service string `json:"service"`
	Tag     string `json:"tag"`
	Changed bool   `json:"changed"`
	Preview bool   `json:"preview,omitempty"`
	Diff    string `json:"diff,omitempty"`
	Sha256  string `json:"sha256"`
}

// unifiedDiff renders a single-hunk unified diff of the change between oldB and
// newB (our machine edits are localized), with up to 3 lines of context. Returns
// "" when they are identical.
func unifiedDiff(oldB, newB []byte, label string) string {
	oldL := strings.Split(string(oldB), "\n")
	newL := strings.Split(string(newB), "\n")

	p := 0
	for p < len(oldL) && p < len(newL) && oldL[p] == newL[p] {
		p++
	}
	s := 0
	for s < len(oldL)-p && s < len(newL)-p && oldL[len(oldL)-1-s] == newL[len(newL)-1-s] {
		s++
	}
	if p == len(oldL) && p == len(newL) {
		return ""
	}

	const ctx = 3
	start := max(0, p-ctx)
	oldChangeEnd, newChangeEnd := len(oldL)-s, len(newL)-s
	oldEnd := min(len(oldL), oldChangeEnd+ctx)
	newEnd := min(len(newL), newChangeEnd+ctx)

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n+++ %s\n", label, label)
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", start+1, oldEnd-start, start+1, newEnd-start)
	for i := start; i < p; i++ {
		fmt.Fprintf(&b, " %s\n", oldL[i])
	}
	for i := p; i < oldChangeEnd; i++ {
		fmt.Fprintf(&b, "-%s\n", oldL[i])
	}
	for i := p; i < newChangeEnd; i++ {
		fmt.Fprintf(&b, "+%s\n", newL[i])
	}
	for i := oldChangeEnd; i < oldEnd; i++ {
		fmt.Fprintf(&b, " %s\n", oldL[i])
	}
	return b.String()
}
