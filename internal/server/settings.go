package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

const settingWebhookURL = "webhook_url"

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

func (a *api) settings(w http.ResponseWriter, r *http.Request) {
	interval := "disabled"
	if a.cfg.CheckInterval > 0 {
		interval = a.cfg.CheckInterval.String()
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
		WebhookURL *string `json:"webhookUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
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
