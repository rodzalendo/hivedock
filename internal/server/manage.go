package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/rogalinski/hivedock/internal/compose"
	"github.com/rogalinski/hivedock/internal/stacks"
)

// deleteStack removes a managed stack: it stops any running containers first
// (so nothing is orphaned), then deletes the stack's directory under
// STACKS_DIR. External stacks are read-only and can't be deleted. Auth + CSRF
// protected. This is destructive — the compose file and everything in the
// stack's directory is removed.
func (a *api) deleteStack(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	st, ok, err := a.stacks.Get(r.Context(), name)
	if err != nil {
		a.logger.Error("delete stack: get", "name", name, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to load stack: "+err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "stack not found: "+name)
		return
	}
	if st.Origin != stacks.OriginManaged || st.Dir == "" {
		writeError(w, http.StatusConflict, "stack is external (read-only); nothing to delete under the stacks directory")
		return
	}

	dir, err := a.childOfStacksDir(st.Dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Serialize with any in-flight deploy for this stack.
	release, acquired := a.runner.Start(name)
	if !acquired {
		writeError(w, http.StatusConflict, "an operation is already running for this stack")
		return
	}
	defer release()

	// Stop and remove containers first so deleting the compose file doesn't
	// orphan a running stack. Only bother if something is actually running.
	if hasRunning(st) && st.ComposeFile != "" {
		op := compose.Op{Stack: name, Action: compose.ActionDown, ComposeFile: st.ComposeFile, ProjectDir: st.Dir}
		if err := a.runner.Exec(r.Context(), op, func(string) {}); err != nil {
			a.logger.Warn("delete stack: down failed", "name", name, "err", err)
			writeError(w, http.StatusInternalServerError, "failed to stop the stack before deleting (stop it first, then retry): "+err.Error())
			return
		}
	}

	if err := os.RemoveAll(dir); err != nil {
		a.logger.Error("delete stack: remove dir", "dir", dir, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to remove stack directory: "+err.Error())
		return
	}

	a.logger.Info("stack deleted", "name", name, "dir", dir)
	a.hub.NotifyChanged("stack:deleted")
	w.WriteHeader(http.StatusNoContent)
}

// renameStack renames a managed stack's directory (its identity + compose
// project name). The stack must be stopped first: renaming a running stack
// would change its compose project name and orphan the live containers, so a
// running stack is rejected with a 409.
func (a *api) renameStack(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body struct {
		NewName string `json:"newName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	newName := strings.TrimSpace(body.NewName)
	if !stackNamePattern.MatchString(newName) {
		writeError(w, http.StatusBadRequest, invalidStackName)
		return
	}
	if newName == name {
		writeError(w, http.StatusBadRequest, "new name is the same as the current name")
		return
	}

	st, ok, err := a.stacks.Get(r.Context(), name)
	if err != nil {
		a.logger.Error("rename stack: get", "name", name, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to load stack: "+err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "stack not found: "+name)
		return
	}
	if st.Origin != stacks.OriginManaged || st.Dir == "" {
		writeError(w, http.StatusConflict, "stack is external (read-only); can't rename")
		return
	}
	if hasRunning(st) {
		writeError(w, http.StatusConflict, "stop the stack before renaming (a running stack's project name can't change without orphaning its containers)")
		return
	}

	oldDir, err := a.childOfStacksDir(st.Dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	root := filepath.Dir(oldDir)
	newDir := filepath.Join(root, newName)
	if filepath.Dir(newDir) != root {
		writeError(w, http.StatusBadRequest, "invalid stack name")
		return
	}
	if _, err := os.Stat(newDir); err == nil {
		writeError(w, http.StatusConflict, "a stack named "+newName+" already exists")
		return
	} else if !os.IsNotExist(err) {
		writeError(w, http.StatusInternalServerError, "failed to check target directory")
		return
	}

	if err := os.Rename(oldDir, newDir); err != nil {
		a.logger.Error("rename stack: rename dir", "from", oldDir, "to", newDir, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to rename stack directory: "+err.Error())
		return
	}

	// Carry the visibility/icon prefs over to the new stack name.
	if a.db != nil {
		if err := a.db.RenameStackPrefs(name, newName); err != nil {
			a.logger.Warn("rename stack: move prefs", "from", name, "to", newName, "err", err)
		}
	}

	composeFile := ""
	if st.ComposeFile != "" {
		composeFile = filepath.Join(newDir, filepath.Base(st.ComposeFile))
	}
	a.logger.Info("stack renamed", "from", name, "to", newName, "dir", newDir)
	a.hub.NotifyChanged("stack:renamed")
	writeJSON(w, http.StatusOK, createStackResponse{Name: newName, Dir: newDir, ComposeFile: composeFile})
}

// childOfStacksDir validates that dir is a direct child of STACKS_DIR — with
// symlinks resolved on both sides (§4.2), so a symlinked stack directory can't
// point the rename/delete at a path outside the tree — and returns its real
// absolute path. Defense in depth against path escapes.
func (a *api) childOfStacksDir(dir string) (string, error) {
	root, err := filepath.EvalSymlinks(a.cfg.StacksDir)
	if err != nil {
		if root, err = filepath.Abs(a.cfg.StacksDir); err != nil {
			return "", err
		}
	}
	abs, err := stacks.Contained(a.cfg.StacksDir, dir)
	if err != nil {
		return "", err
	}
	if abs == root || filepath.Dir(abs) != root {
		return "", fmt.Errorf("refusing to operate on %q: not a stack under the stacks directory", dir)
	}
	return abs, nil
}

// hasRunning reports whether any of a stack's services has a running container.
func hasRunning(st stacks.Stack) bool {
	for _, svc := range st.Services {
		if svc.State == "running" {
			return true
		}
	}
	return false
}
