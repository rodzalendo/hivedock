package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/rogalinski/hivedock/internal/registry"
	"github.com/rogalinski/hivedock/internal/store"
)

// settingRegistryConfig holds the per-registry credential/TLS map as JSON (§6.1,
// §6.2). Stored under DATA_DIR file permissions; at-rest encryption is out of
// scope (stated in THREAT_MODEL.md).
const settingRegistryConfig = "registry_config"

// registryConfig is one registry host's stored settings.
type registryConfig struct {
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	CABundlePath string `json:"caBundlePath,omitempty"`
	Insecure     bool   `json:"insecure,omitempty"`
}

// registryConfigView is the safe shape returned to the UI — the password is
// never echoed, only whether one is set.
type registryConfigView struct {
	Host         string `json:"host"`
	Username     string `json:"username,omitempty"`
	HasPassword  bool   `json:"hasPassword"`
	CABundlePath string `json:"caBundlePath,omitempty"`
	Insecure     bool   `json:"insecure,omitempty"`
}

func loadRegistryConfigs(db *store.Store) map[string]registryConfig {
	out := map[string]registryConfig{}
	if db == nil {
		return out
	}
	if v, ok, err := db.GetSetting(settingRegistryConfig); err == nil && ok {
		_ = json.Unmarshal([]byte(v), &out)
	}
	return out
}

// registryConfigResolver adapts the stored config map into the registry client's
// per-host resolver (read fresh from the store each call, so edits apply live).
func registryConfigResolver(db *store.Store) registry.ConfigResolver {
	return func(host string) registry.HostConfig {
		c := loadRegistryConfigs(db)[host]
		return registry.HostConfig{
			Username:     c.Username,
			Password:     c.Password,
			CABundlePath: c.CABundlePath,
			Insecure:     c.Insecure,
		}
	}
}

// listRegistries returns the configured registries (passwords masked).
func (a *api) listRegistries(w http.ResponseWriter, r *http.Request) {
	cfgs := loadRegistryConfigs(a.db)
	views := make([]registryConfigView, 0, len(cfgs))
	for host, c := range cfgs {
		views = append(views, registryConfigView{
			Host: host, Username: c.Username, HasPassword: c.Password != "",
			CABundlePath: c.CABundlePath, Insecure: c.Insecure,
		})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Host < views[j].Host })
	writeJSON(w, http.StatusOK, views)
}

// putRegistry upserts one registry's config. An empty password field on an
// existing host keeps the stored password (so the UI needn't re-send secrets).
func (a *api) putRegistry(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	var body struct {
		Host         string `json:"host"`
		Username     string `json:"username"`
		Password     string `json:"password"`
		CABundlePath string `json:"caBundlePath"`
		Insecure     bool   `json:"insecure"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	host := strings.TrimSpace(strings.ToLower(body.Host))
	if host == "" || strings.ContainsAny(host, "/ ") {
		writeError(w, http.StatusBadRequest, "host is required and must be a bare registry hostname (e.g. registry.example.com)")
		return
	}
	cfgs := loadRegistryConfigs(a.db)
	existing := cfgs[host]
	pw := body.Password
	if pw == "" {
		pw = existing.Password // keep the stored secret when left blank
	}
	cfgs[host] = registryConfig{
		Username:     strings.TrimSpace(body.Username),
		Password:     pw,
		CABundlePath: strings.TrimSpace(body.CABundlePath),
		Insecure:     body.Insecure,
	}
	if err := a.saveRegistryConfigs(cfgs); err != nil {
		a.logger.Error("registry config: save", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save registry config")
		return
	}
	a.logger.Info("registry config saved", "host", host, "hasCreds", pw != "", "insecure", body.Insecure)
	a.listRegistries(w, r)
}

// deleteRegistry removes one registry's config (?host=…).
func (a *api) deleteRegistry(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	host := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("host")))
	if host == "" {
		writeError(w, http.StatusBadRequest, "host query parameter is required")
		return
	}
	cfgs := loadRegistryConfigs(a.db)
	delete(cfgs, host)
	if err := a.saveRegistryConfigs(cfgs); err != nil {
		a.logger.Error("registry config: delete", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to update registry config")
		return
	}
	a.logger.Info("registry config removed", "host", host)
	a.listRegistries(w, r)
}

func (a *api) saveRegistryConfigs(cfgs map[string]registryConfig) error {
	if len(cfgs) == 0 {
		return a.db.DeleteSetting(settingRegistryConfig)
	}
	b, err := json.Marshal(cfgs)
	if err != nil {
		return err
	}
	return a.db.SetSetting(settingRegistryConfig, string(b))
}
