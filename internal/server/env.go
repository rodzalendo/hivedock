package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
)

type envFileResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Exists  bool   `json:"exists"`
}

// getEnv returns a managed stack's .env file. A stack often has none yet, so a
// missing file is a normal 200 with empty content and exists:false (not a 404) —
// the editor opens blank and a save creates it.
func (a *api) getEnv(w http.ResponseWriter, r *http.Request) {
	st, ok := a.managedComposeFile(w, r)
	if !ok {
		return
	}
	path, ok := a.containedPath(w, filepath.Join(st.Dir, ".env"))
	if !ok {
		return
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusOK, envFileResponse{Path: path, Content: "", Exists: false})
		return
	}
	if err != nil {
		a.logger.Error("env: read file", "path", path, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to read .env: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envFileResponse{Path: path, Content: string(data), Exists: true})
}

// putEnv atomically writes a managed stack's .env file, creating it if needed.
// Like the compose editor: save ≠ deploy — the change applies on the next
// deploy (and shows as drift until then). No `docker compose config` validation
// is needed; .env is plain KEY=VALUE that compose interpolates at runtime.
func (a *api) putEnv(w http.ResponseWriter, r *http.Request) {
	st, ok := a.managedComposeFile(w, r)
	if !ok {
		return
	}
	content, ok := readContentBody(w, r)
	if !ok {
		return
	}
	path, ok := a.containedPath(w, filepath.Join(st.Dir, ".env"))
	if !ok {
		return
	}
	if err := atomicWrite(path, content); err != nil {
		a.logger.Error("env: write file", "path", path, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save .env: "+err.Error())
		return
	}
	a.logger.Info("env saved", "stack", st.Name, "path", path, "bytes", len(content))
	a.hub.NotifyChanged("env:saved")
	writeJSON(w, http.StatusOK, envFileResponse{Path: path, Content: string(content), Exists: true})
}
