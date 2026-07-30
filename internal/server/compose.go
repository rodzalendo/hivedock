package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// maxComposeBytes caps the compose/.env editor payload. These files are tiny;
// this is just an abuse guard.
const maxComposeBytes = 1 << 20 // 1 MiB

// getCompose returns the raw compose file text for a managed stack.
func (a *api) getCompose(w http.ResponseWriter, r *http.Request) {
	be, err := a.backendFor(hostParam(r))
	if err != nil {
		a.httpError(w, err)
		return
	}
	cf, err := be.GetCompose(r.Context(), chi.URLParam(r, "name"))
	if err != nil {
		a.httpError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cf)
}

// validateCompose checks a candidate compose body via `docker compose config`
// without writing anything. Returns {valid:true} or {valid:false, error:"…"}.
func (a *api) validateCompose(w http.ResponseWriter, r *http.Request) {
	be, err := a.backendFor(hostParam(r))
	if err != nil {
		a.httpError(w, err)
		return
	}
	content, _, ok := readContentBody(w, r)
	if !ok {
		return
	}
	v, err := be.ValidateCompose(r.Context(), chi.URLParam(r, "name"), content)
	if err != nil {
		a.httpError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// putCompose validates then atomically writes a managed stack's compose file.
// Save ≠ deploy: this only updates the file on disk. The running containers are
// untouched, so drift will surface until the user deploys.
func (a *api) putCompose(w http.ResponseWriter, r *http.Request) {
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
	cf, err := be.PutCompose(r.Context(), name, content, baseSha)
	if err != nil {
		a.httpError(w, err)
		return
	}
	a.logger.Info("compose saved", "host", hostParam(r), "stack", name, "bytes", len(content))
	a.hub.NotifyChanged("compose:saved")
	writeJSON(w, http.StatusOK, cf)
}

// readContentBody reads a {content, baseSha256} JSON body up to the size cap.
// baseSha256 is the hash the client loaded (optimistic-lock check on save, §5.1);
// it is empty for callers that don't lock (e.g. validate). Shared by the compose
// and .env editors.
func readContentBody(w http.ResponseWriter, r *http.Request) (content []byte, baseSha string, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxComposeBytes)
	var body struct {
		Content    string `json:"content"`
		BaseSha256 string `json:"baseSha256"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if _, isMax := err.(*http.MaxBytesError); isMax {
			writeError(w, http.StatusRequestEntityTooLarge, "file too large")
			return nil, "", false
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return nil, "", false
	}
	return []byte(body.Content), body.BaseSha256, true
}
