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
	Sha256  string `json:"sha256"` // hash of Content ("" hashes empty); present on save (§5.1)
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
		// No file yet: hash of empty so a save can still lock against creation.
		writeJSON(w, http.StatusOK, envFileResponse{Path: path, Content: "", Exists: false, Sha256: sha256hex(nil)})
		return
	}
	if err != nil {
		a.logger.Error("env: read file", "path", path, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to read .env: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envFileResponse{Path: path, Content: string(data), Exists: true, Sha256: sha256hex(data)})
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
	content, baseSha, ok := readContentBody(w, r)
	if !ok {
		return
	}
	path, ok := a.containedPath(w, filepath.Join(st.Dir, ".env"))
	if !ok {
		return
	}
	if !a.checkOptimisticLock(w, path, baseSha) {
		return
	}
	action := "save " + st.Name + " .env"
	if !a.gitSnapshotBefore(w, action) {
		return
	}
	if err := atomicWrite(path, content); err != nil {
		a.logger.Error("env: write file", "path", path, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save .env: "+err.Error())
		return
	}
	if err := a.gitCommitAfter(action); err != nil {
		a.logger.Error("env: git commit", "err", err)
		writeError(w, http.StatusInternalServerError, "saved, but the git commit failed: "+err.Error())
		return
	}
	a.logger.Info("env saved", "stack", st.Name, "path", path, "bytes", len(content))
	a.hub.NotifyChanged("env:saved")
	writeJSON(w, http.StatusOK, envFileResponse{Path: path, Content: string(content), Exists: true, Sha256: sha256hex(content)})
}
