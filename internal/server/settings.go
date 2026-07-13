package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	settingCheckInterval = "check_interval" // "off" or a Go duration; overrides CHECK_INTERVAL
)

type settingsResponse struct {
	StacksDir     string `json:"stacksDir"`
	DataDir       string `json:"dataDir"`
	CheckInterval string `json:"checkInterval"` // human duration, or "disabled"
	PublicHost    string `json:"publicHost"`
	AuthDisabled  bool   `json:"authDisabled"`
	Version       string `json:"version"`
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
	writeJSON(w, http.StatusOK, settingsResponse{
		StacksDir:     a.cfg.StacksDir,
		DataDir:       a.cfg.DataDir,
		CheckInterval: interval,
		PublicHost:    a.cfg.PublicHost,
		AuthDisabled:  a.cfg.AuthDisabled,
		Version:       version,
	})
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
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
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
