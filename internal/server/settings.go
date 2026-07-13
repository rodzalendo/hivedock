package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	settingWebhookURL    = "webhook_url"
	settingCheckInterval = "check_interval" // "off" or a Go duration; overrides CHECK_INTERVAL
)

type settingsResponse struct {
	StacksDir      string `json:"stacksDir"`
	DataDir        string `json:"dataDir"`
	CheckInterval  string `json:"checkInterval"` // human duration, or "disabled"
	WebhookURL     string `json:"webhookUrl"`
	WebhookFromEnv bool   `json:"webhookFromEnv"` // set via env (still overridable in-app)
	PublicHost     string `json:"publicHost"`
	AuthDisabled   bool   `json:"authDisabled"`
	Version        string `json:"version"`
}

// effectiveWebhookURL resolves the webhook URL: an in-app override (settings
// table) wins over the WEBHOOK_URL env default.
func (a *api) effectiveWebhookURL() string {
	if a.db != nil {
		if v, ok, err := a.db.GetSetting(settingWebhookURL); err == nil && ok {
			return v
		}
	}
	return a.cfg.WebhookURL
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
	_, overridden := a.settingOverride(settingWebhookURL)
	writeJSON(w, http.StatusOK, settingsResponse{
		StacksDir:      a.cfg.StacksDir,
		DataDir:        a.cfg.DataDir,
		CheckInterval:  interval,
		WebhookURL:     a.effectiveWebhookURL(),
		WebhookFromEnv: a.cfg.WebhookURL != "" && !overridden,
		PublicHost:     a.cfg.PublicHost,
		AuthDisabled:   a.cfg.AuthDisabled,
		Version:        version,
	})
}

func (a *api) settingOverride(key string) (string, bool) {
	if a.db == nil {
		return "", false
	}
	v, ok, _ := a.db.GetSetting(key)
	return v, ok
}

// updateSettings persists the editable settings (currently the webhook URL). An
// empty webhookUrl clears the override, reverting to the env default.
func (a *api) updateSettings(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	var body struct {
		WebhookURL    *string `json:"webhookUrl"`
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
	if body.WebhookURL != nil {
		url := strings.TrimSpace(*body.WebhookURL)
		if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			writeError(w, http.StatusBadRequest, "webhook URL must start with http:// or https://")
			return
		}
		if url == "" {
			if err := a.db.DeleteSetting(settingWebhookURL); err != nil {
				a.logger.Error("settings: clear webhook", "err", err)
				writeError(w, http.StatusInternalServerError, "failed to save settings")
				return
			}
		} else if err := a.db.SetSetting(settingWebhookURL, url); err != nil {
			a.logger.Error("settings: set webhook", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to save settings")
			return
		}
	}
	a.settings(w, r)
}

// testWebhook POSTs a sample payload to the given URL (or the configured one
// when the body omits it) so the user can verify their notification wiring
// without waiting for a real update. Synchronous — reports the upstream result.
func (a *api) testWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // empty/missing body = use saved URL
	url := strings.TrimSpace(body.URL)
	if url == "" {
		url = a.effectiveWebhookURL()
	}
	if url == "" {
		writeError(w, http.StatusBadRequest, "no webhook URL to test — enter one first")
		return
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		writeError(w, http.StatusBadRequest, "webhook URL must start with http:// or https://")
		return
	}
	payload := map[string]any{
		"event":   "test",
		"time":    time.Now().UTC().Format(time.RFC3339),
		"count":   0,
		"updates": []any{},
		"message": "HiveDock webhook test — notifications are wired up.",
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build test payload")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid webhook URL")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "webhook unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("webhook endpoint responded with HTTP %d", resp.StatusCode))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": resp.StatusCode})
}
