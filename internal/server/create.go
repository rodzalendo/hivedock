package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// stackNamePattern constrains a new stack's directory name to a single, safe
// path segment: starts alphanumeric, then alphanumerics plus . _ -, max 64. No
// separators and no way to spell "..", so it can't escape STACKS_DIR.
var stackNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// composeTemplate is the starter compose.yaml written for a new stack. It is
// valid and deployable as-is (nginx), but meant to be edited on the Compose tab.
const composeTemplate = `# %s — starter compose file. Edit on the Compose tab, then Save and Deploy.
services:
  app:
    image: nginx:alpine
    restart: unless-stopped
    ports:
      - "8080:80"
`

type createStackResponse struct {
	Name        string `json:"name"`
	Dir         string `json:"dir"`
	ComposeFile string `json:"composeFile"`
}

// createStack scaffolds a new managed stack: a directory under STACKS_DIR plus a
// template compose.yaml. It does not deploy — the user edits then deploys. Auth
// + CSRF protected (mutating POST in the guarded group).
func (a *api) createStack(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if !stackNamePattern.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid stack name: use letters, digits, dot, dash or underscore (max 64)")
		return
	}

	root, err := filepath.Abs(a.cfg.StacksDir)
	if err != nil {
		a.logger.Error("create stack: resolve stacks dir", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to resolve stacks directory")
		return
	}
	dir := filepath.Join(root, name)
	// Defense in depth: the resolved dir must be a direct child of the root.
	if filepath.Dir(dir) != root {
		writeError(w, http.StatusBadRequest, "invalid stack name")
		return
	}

	if _, err := os.Stat(dir); err == nil {
		writeError(w, http.StatusConflict, "a stack named "+name+" already exists")
		return
	} else if !os.IsNotExist(err) {
		a.logger.Error("create stack: stat dir", "dir", dir, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to check stack directory")
		return
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.logger.Error("create stack: mkdir", "dir", dir, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to create stack directory: "+err.Error())
		return
	}
	composeFile := filepath.Join(dir, "compose.yaml")
	content := fmt.Sprintf(composeTemplate, name)
	if err := os.WriteFile(composeFile, []byte(content), 0o644); err != nil {
		// Roll back the (empty) directory so a half-created stack isn't left behind.
		_ = os.Remove(dir)
		a.logger.Error("create stack: write compose", "path", composeFile, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to write compose file: "+err.Error())
		return
	}

	a.logger.Info("stack created", "name", name, "dir", dir)
	a.hub.NotifyChanged("stack:created")
	writeJSON(w, http.StatusCreated, createStackResponse{Name: name, Dir: dir, ComposeFile: composeFile})
}
