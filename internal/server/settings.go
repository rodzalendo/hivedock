package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/rogalinski/hivedock/internal/stacks"
)

const (
	settingCheckInterval = "check_interval" // "off" or a Go duration; overrides CHECK_INTERVAL
	settingGitAutoCommit = "git_autocommit" // "1" to keep a local git history of stack changes (§5.4)
)

type settingsResponse struct {
	StacksDir     string `json:"stacksDir"`
	DataDir       string `json:"dataDir"`
	CheckInterval string `json:"checkInterval"` // human duration, or "disabled"
	PublicHost    string `json:"publicHost"`
	AuthMode      string `json:"authMode"`      // "password" | "trusted header (forward auth)"
	UpdateMode    string `json:"updateMode"`    // full | check-only | off (§3.4)
	GitAutoCommit bool   `json:"gitAutoCommit"` // §5.4 local audit trail
	GitWorktree   bool   `json:"gitWorktree"`   // whether STACKS_DIR is a git repo (drives the "initialize" UI)
	Version       string `json:"version"`
}

// gitAutoCommitEnabled reports whether §5.4 git auto-commit is on.
func (a *api) gitAutoCommitEnabled() bool {
	if a.db == nil {
		return false
	}
	v, ok, err := a.db.GetSetting(settingGitAutoCommit)
	return err == nil && ok && v == "1"
}

// gitSnapshotBefore commits any pending out-of-band changes under STACKS_DIR as a
// snapshot, before a HiveDock-initiated write (§5.4). No-op when auto-commit is
// off. On failure it writes the error response and returns false so the caller
// aborts the write — a broken paper trail stops the press.
func (a *api) gitSnapshotBefore(w http.ResponseWriter, action string) bool {
	if !a.gitAutoCommitEnabled() {
		return true
	}
	if err := stacks.GitCommitAll(a.cfg.StacksDir, "snapshot before "+action); err != nil {
		a.logger.Error("git snapshot before write", "action", action, "err", err)
		writeError(w, http.StatusInternalServerError,
			"git snapshot failed — not saving (git auto-commit is on): "+err.Error())
		return false
	}
	return true
}

// gitCommitAfter commits a completed HiveDock write. No-op when auto-commit is
// off. The file is already on disk, so the caller surfaces any error rather than
// rolling back.
func (a *api) gitCommitAfter(action string) error {
	if !a.gitAutoCommitEnabled() {
		return nil
	}
	return stacks.GitCommitAll(a.cfg.StacksDir, action)
}

// effectiveCheckInterval resolves the update-check cadence: an in-app override
// wins over the CHECK_INTERVAL env default. 0 = disabled. The scheduler reads
// this each tick, so changes apply without a restart.
func (a *api) effectiveCheckInterval() time.Duration {
	if a.db != nil {
		if v, ok, err := a.db.GetSetting(settingCheckInterval); err == nil && ok {
			if strings.EqualFold(v, "off") {
				return 0
			}
			if d, err := time.ParseDuration(v); err == nil {
				return d
			}
		}
	}
	return a.cfg.CheckInterval
}

func (a *api) settings(w http.ResponseWriter, r *http.Request) {
	interval := "disabled"
	if iv := a.effectiveCheckInterval(); iv > 0 {
		// Tidy Go's duration string: "30m0s" -> "30m", "6h0m0s" -> "6h".
		s := iv.String()
		s = strings.TrimSuffix(s, "0s")
		s = strings.TrimSuffix(s, "0m")
		if s == "" {
			s = iv.String()
		}
		interval = s
	}
	authMode := "password"
	if a.cfg.TrustedHeader != "" {
		authMode = "trusted header (forward auth)"
	}
	writeJSON(w, http.StatusOK, settingsResponse{
		StacksDir:     a.cfg.StacksDir,
		DataDir:       a.cfg.DataDir,
		CheckInterval: interval,
		PublicHost:    a.cfg.PublicHost,
		AuthMode:      authMode,
		UpdateMode:    a.appUpdateMode(),
		GitAutoCommit: a.gitAutoCommitEnabled(),
		GitWorktree:   stacks.IsGitWorktree(a.cfg.StacksDir),
		Version:       version,
	})
}

// gitInit initializes STACKS_DIR as a git repository (the one-click "initialize"
// behind the git auto-commit toggle, §5.4). Local only — no remotes.
func (a *api) gitInit(w http.ResponseWriter, r *http.Request) {
	if err := stacks.GitInit(a.cfg.StacksDir); err != nil {
		a.logger.Error("git init", "dir", a.cfg.StacksDir, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to initialize git repo: "+err.Error())
		return
	}
	a.settings(w, r)
}

// updateSettings persists the editable settings (currently the update-check
// interval).
func (a *api) updateSettings(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	var body struct {
		CheckInterval *string `json:"checkInterval"` // "off", or a Go duration >= 5m; "" reverts to env
		UpdateMode    *string `json:"updateMode"`    // full | check-only | off (§3.4)
		GitAutoCommit *bool   `json:"gitAutoCommit"` // §5.4 local audit trail
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.GitAutoCommit != nil {
		if *body.GitAutoCommit && !stacks.IsGitWorktree(a.cfg.StacksDir) {
			// Enabling requires a worktree; the UI calls /git-init first.
			writeError(w, http.StatusConflict, "STACKS_DIR is not a git repository — initialize it first")
			return
		}
		val := "0"
		if *body.GitAutoCommit {
			val = "1"
		}
		if err := a.db.SetSetting(settingGitAutoCommit, val); err != nil {
			a.logger.Error("settings: set git auto-commit", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to save settings")
			return
		}
	}
	if body.UpdateMode != nil {
		v := strings.TrimSpace(strings.ToLower(*body.UpdateMode))
		if !validUpdateMode(v) {
			writeError(w, http.StatusBadRequest, "update mode must be one of: full, check-only, off")
			return
		}
		if err := a.db.SetSetting(settingAppUpdateMode, v); err != nil {
			a.logger.Error("settings: set update mode", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to save settings")
			return
		}
	}
	if body.CheckInterval != nil {
		v := strings.TrimSpace(strings.ToLower(*body.CheckInterval))
		switch {
		case v == "":
			if err := a.db.DeleteSetting(settingCheckInterval); err != nil {
				a.logger.Error("settings: clear check interval", "err", err)
				writeError(w, http.StatusInternalServerError, "failed to save settings")
				return
			}
		case v == "off":
			if err := a.db.SetSetting(settingCheckInterval, "off"); err != nil {
				a.logger.Error("settings: set check interval", "err", err)
				writeError(w, http.StatusInternalServerError, "failed to save settings")
				return
			}
		default:
			d, err := time.ParseDuration(v)
			if err != nil || d < 5*time.Minute {
				writeError(w, http.StatusBadRequest, "check interval must be 'off' or a duration of at least 5m (e.g. 30m, 6h)")
				return
			}
			if err := a.db.SetSetting(settingCheckInterval, d.String()); err != nil {
				a.logger.Error("settings: set check interval", "err", err)
				writeError(w, http.StatusInternalServerError, "failed to save settings")
				return
			}
		}
	}
	a.settings(w, r)
}
