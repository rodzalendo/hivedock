package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/rogalinski/hivedock/internal/compose"
	"github.com/rogalinski/hivedock/internal/stacks"
)

// maxComposeBytes caps the compose file editor payload. Compose files are tiny;
// this is just an abuse guard.
const maxComposeBytes = 1 << 20 // 1 MiB

type composeFileResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// managedComposeFile resolves a managed stack's compose file path, writing the
// appropriate error response and returning ok=false when the stack is missing
// or read-only. The path comes from the scanner (never from raw user input), so
// there is no path-traversal surface.
func (a *api) managedComposeFile(w http.ResponseWriter, r *http.Request) (st stacks.Stack, ok bool) {
	name := chi.URLParam(r, "name")
	st, found, err := a.stacks.Get(r.Context(), name)
	if err != nil {
		a.logger.Error("compose: get stack", "name", name, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to load stack: "+err.Error())
		return st, false
	}
	if !found {
		writeError(w, http.StatusNotFound, "stack not found: "+name)
		return st, false
	}
	if st.Origin != stacks.OriginManaged || st.ComposeFile == "" {
		writeError(w, http.StatusConflict, "stack is external (read-only); no compose file to edit")
		return st, false
	}
	return st, true
}

// containedPath resolves p (a file or dir belonging to a stack) with symlinks
// and requires it to stay inside STACKS_DIR (§4.2). It writes a 400 and returns
// ok=false on escape, so a compose/.env that symlinks out of the tree is refused
// even though the scanner would otherwise follow it. The real (resolved) path is
// what callers should read/write.
func (a *api) containedPath(w http.ResponseWriter, p string) (string, bool) {
	real, err := stacks.Contained(a.cfg.StacksDir, p)
	if err != nil {
		a.logger.Warn("path containment refused", "path", p, "err", err)
		writeError(w, http.StatusBadRequest, "refusing to access a path outside the stacks directory")
		return "", false
	}
	return real, true
}

// getCompose returns the raw compose file text for a managed stack.
func (a *api) getCompose(w http.ResponseWriter, r *http.Request) {
	st, ok := a.managedComposeFile(w, r)
	if !ok {
		return
	}
	real, ok := a.containedPath(w, st.ComposeFile)
	if !ok {
		return
	}
	data, err := os.ReadFile(real)
	if err != nil {
		a.logger.Error("compose: read file", "path", st.ComposeFile, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to read compose file: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, composeFileResponse{Path: st.ComposeFile, Content: string(data)})
}

// validateCompose checks a candidate compose body via `docker compose config`
// without writing anything. Returns {valid:true} or {valid:false, error:"…"}.
func (a *api) validateCompose(w http.ResponseWriter, r *http.Request) {
	st, ok := a.managedComposeFile(w, r)
	if !ok {
		return
	}
	content, ok := readContentBody(w, r)
	if !ok {
		return
	}
	if err := compose.Validate(r.Context(), st.Dir, content); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true})
}

// putCompose validates then atomically writes a managed stack's compose file.
// Save ≠ deploy: this only updates the file on disk. The running containers are
// untouched, so drift will surface until the user deploys.
func (a *api) putCompose(w http.ResponseWriter, r *http.Request) {
	st, ok := a.managedComposeFile(w, r)
	if !ok {
		return
	}
	content, ok := readContentBody(w, r)
	if !ok {
		return
	}
	real, ok := a.containedPath(w, st.ComposeFile)
	if !ok {
		return
	}
	// Validate before touching disk — a bad draft must never clobber the file.
	if err := compose.Validate(r.Context(), st.Dir, content); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := atomicWrite(real, content); err != nil {
		a.logger.Error("compose: write file", "path", st.ComposeFile, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save compose file: "+err.Error())
		return
	}
	a.logger.Info("compose saved", "stack", st.Name, "path", st.ComposeFile, "bytes", len(content))
	a.hub.NotifyChanged("compose:saved")
	writeJSON(w, http.StatusOK, composeFileResponse{Path: st.ComposeFile, Content: string(content)})
}

// readContentBody reads a {content:"…"} JSON body up to the size cap. Shared by
// the compose and .env editors.
func readContentBody(w http.ResponseWriter, r *http.Request) (content []byte, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxComposeBytes)
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if _, isMax := err.(*http.MaxBytesError); isMax {
			writeError(w, http.StatusRequestEntityTooLarge, "file too large")
			return nil, false
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return nil, false
	}
	return []byte(body.Content), true
}

// atomicWrite writes data to a temp file in the target's directory and renames
// it over path, preserving the existing file's mode. The rename is atomic on the
// same filesystem, so a reader (or the daemon) never sees a half-written file.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode()
	}
	tmp, err := os.CreateTemp(dir, ".hivedock-compose-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
