package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/rogalinski/hivedock/internal/compose"
)

// updateService rewrites one service's image tag in a managed stack's compose
// file (comment-preserving) and saves it. Save ≠ deploy: the caller redeploys
// separately (the Updates view triggers `compose up` per affected stack). Env-
// interpolated and digest-pinned images are surfaced (409), never rewritten.
func (a *api) updateService(w http.ResponseWriter, r *http.Request) {
	st, ok := a.managedComposeFile(w, r)
	if !ok {
		return
	}
	service := chi.URLParam(r, "service")

	var body struct {
		Tag string `json:"tag"`
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

	content, err := os.ReadFile(st.ComposeFile)
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

	if err := atomicWrite(st.ComposeFile, updated); err != nil {
		a.logger.Error("update service: write compose", "path", st.ComposeFile, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save compose file: "+err.Error())
		return
	}
	a.logger.Info("service image updated", "stack", st.Name, "service", service, "tag", body.Tag)
	a.hub.NotifyChanged("update:" + st.Name)
	writeJSON(w, http.StatusOK, map[string]string{"stack": st.Name, "service": service, "tag": body.Tag})
}
