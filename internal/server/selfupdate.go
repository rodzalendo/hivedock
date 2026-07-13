package server

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/rogalinski/hivedock/internal/registry"
)

// selfImage is the repository HiveDock is distributed from; the self-update
// check compares the running version against its published tags.
const selfImage = "ghcr.io/rodzalendo/hivedock"

// appUpdateResponse tells the sidebar whether a newer HiveDock release exists.
type appUpdateResponse struct {
	Current   string `json:"current"`
	Candidate string `json:"candidate,omitempty"`
	HasUpdate bool   `json:"hasUpdate"`
	Checkable bool   `json:"checkable"` // false for dev/edge builds (no semver to compare)
	NotesURL  string `json:"notesUrl,omitempty"`
}

// selfCheckCache memoizes the registry lookup so page reloads (the UI checks
// on every load) don't hammer ghcr.
type selfCheckCache struct {
	mu   sync.Mutex
	at   time.Time
	resp appUpdateResponse
}

// appUpdate reports whether a newer release than the running one is published.
func (a *api) appUpdate(w http.ResponseWriter, r *http.Request) {
	resp := appUpdateResponse{Current: version}
	if !registry.IsVersion(version) {
		// dev / edge builds have no semver to compare against.
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp.Checkable = true

	a.selfCheck.mu.Lock()
	if time.Since(a.selfCheck.at) < 15*time.Minute {
		cached := a.selfCheck.resp
		a.selfCheck.mu.Unlock()
		writeJSON(w, http.StatusOK, cached)
		return
	}
	a.selfCheck.mu.Unlock()

	res := a.checker.CheckImage(r.Context(), selfImage+":"+version)
	if res.HasUpdate && res.Candidate != "" {
		resp.HasUpdate = true
		resp.Candidate = res.Candidate
		resp.NotesURL = "https://github.com/rodzalendo/hivedock/releases/tag/v" + res.Candidate
	}

	a.selfCheck.mu.Lock()
	a.selfCheck.at = time.Now()
	a.selfCheck.resp = resp
	a.selfCheck.mu.Unlock()
	writeJSON(w, http.StatusOK, resp)
}

// selfUpdate replaces the running HiveDock container with the newest image.
// A container cannot `compose up` itself — it would be killed halfway through
// its own recreate — so this launches a small DETACHED helper container (from
// the already-present HiveDock image, which ships the docker CLI) that runs
// `docker compose pull && up -d` against HiveDock's own compose project and
// survives this process being replaced. The UI polls /api/health until the
// new version answers.
func (a *api) selfUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.selfUpdating.CompareAndSwap(false, true) {
		writeError(w, http.StatusConflict, "a self-update is already in progress")
		return
	}
	// If the update succeeds this process is replaced and the flag dies with
	// it; if the helper fails out-of-band, let the user retry after a while.
	time.AfterFunc(5*time.Minute, func() { a.selfUpdating.Store(false) })

	workdir, cfgFile, image, err := selfComposeProject()
	if err != nil {
		a.selfUpdating.Store(false)
		writeError(w, http.StatusConflict, "cannot self-update: "+err.Error())
		return
	}

	script := fmt.Sprintf(
		"docker compose --ansi never -f %q --project-directory %q pull && docker compose --ansi never -f %q --project-directory %q up -d",
		cfgFile, workdir, cfgFile, workdir,
	)
	cmd := exec.CommandContext(r.Context(), "docker", "run", "-d", "--rm",
		"--name", "hivedock-self-update",
		"--entrypoint", "sh",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"-v", workdir+":"+workdir,
		image, "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		a.selfUpdating.Store(false)
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		a.logger.Error("self-update: launch helper", "err", err, "out", msg)
		writeError(w, http.StatusInternalServerError, "failed to launch updater: "+msg)
		return
	}
	a.logger.Info("self-update helper launched", "container", strings.TrimSpace(string(out)))
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "updating"})
}

// selfComposeProject discovers the running HiveDock container's own compose
// project — host-side working dir, compose file, and image — from the labels
// docker compose stamps on every container it creates. Errors when HiveDock
// isn't running as a compose-managed container (dev builds, plain docker run).
func selfComposeProject() (workdir, cfgFile, image string, err error) {
	hostname, _ := os.Hostname() // inside a container this is the container id
	for _, target := range []string{hostname, "hivedock"} {
		if target == "" {
			continue
		}
		out, ierr := exec.Command("docker", "inspect", "--format",
			`{{index .Config.Labels "com.docker.compose.project.working_dir"}}|{{index .Config.Labels "com.docker.compose.project.config_files"}}|{{.Config.Image}}`,
			target).Output()
		if ierr != nil {
			err = fmt.Errorf("inspect %s: %w", target, ierr)
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 3)
		if len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != "" {
			// config_files may list several, comma-separated; the first is the
			// project's main compose file.
			return parts[0], strings.Split(parts[1], ",")[0], parts[2], nil
		}
		err = fmt.Errorf("container %q has no compose labels — was HiveDock started with docker compose?", target)
	}
	return "", "", "", err
}
