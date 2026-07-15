package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/rogalinski/hivedock/internal/discovery"
)

// listHome resolves the zero-config homepage cards from the current truth model.
func (a *api) listHome(w http.ResponseWriter, r *http.Request) {
	stacksList, err := a.stacks.List(r.Context())
	if err != nil {
		a.logger.Error("home: list stacks", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to build homepage: "+err.Error())
		return
	}

	overrides, err := a.hiddenOverrides()
	if err != nil {
		a.logger.Warn("home: load hidden overrides", "err", err)
	}
	iconOverrides, err := a.iconOverrides()
	if err != nil {
		a.logger.Warn("home: load icon overrides", "err", err)
	}
	nameOverrides, err := a.nameOverrides()
	if err != nil {
		a.logger.Warn("home: load name overrides", "err", err)
	}
	urlOverrides, err := a.urlOverrides()
	if err != nil {
		a.logger.Warn("home: load url overrides", "err", err)
	}

	entries := discovery.Resolve(stacksList, discovery.Options{
		Host:           a.publicHost(r),
		HiddenOverride: overrides,
		IconOverride:   iconOverrides,
		NameOverride:   nameOverrides,
		URLOverride:    urlOverrides,
	})
	if entries == nil {
		entries = []discovery.Entry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// setVisibility persists a user's hide/unhide for a service (the sidecar
// auto-hide override). NB: when auth lands (Phase 3) this preference write goes
// behind it like any other mutation.
func (a *api) setVisibility(w http.ResponseWriter, r *http.Request) {
	stack := chi.URLParam(r, "stack")
	service := chi.URLParam(r, "service")
	var body struct {
		Hidden bool `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	if err := a.db.SetServiceHidden(stack, service, body.Hidden); err != nil {
		a.logger.Error("set visibility", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to persist preference")
		return
	}
	a.hub.NotifyChanged("prefs")
	w.WriteHeader(http.StatusNoContent)
}

// homeLayoutKey is the settings key holding the dashboard layout preferences
// (column count, sort, group order, custom group titles). The value is an
// opaque JSON object owned by the frontend; the server only validates shape
// and size.
const homeLayoutKey = "home_layout"

// getHomeLayout returns the saved dashboard layout, or {} when none is set.
func (a *api) getHomeLayout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if a.db == nil {
		_, _ = w.Write([]byte("{}"))
		return
	}
	v, ok, err := a.db.GetSetting(homeLayoutKey)
	if err != nil {
		a.logger.Warn("home layout: load", "err", err)
	}
	if !ok || v == "" {
		_, _ = w.Write([]byte("{}"))
		return
	}
	_, _ = w.Write([]byte(v))
}

// putHomeLayout stores the dashboard layout. The body must be a JSON object
// (bounded in size); its fields are the frontend's business.
func (a *api) putHomeLayout(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 16*1024+1))
	if err != nil || len(body) > 16*1024 {
		writeError(w, http.StatusBadRequest, "layout too large (max 16KB)")
		return
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		writeError(w, http.StatusBadRequest, "layout must be a JSON object")
		return
	}
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	if err := a.db.SetSetting(homeLayoutKey, string(body)); err != nil {
		a.logger.Error("home layout: save", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save layout")
		return
	}
	a.hub.NotifyChanged("prefs")
	w.WriteHeader(http.StatusNoContent)
}

// setIcon persists a user's custom icon (URL or dashboard-icons slug) for a
// service; an empty value clears it and reverts to the automatic icon.
func (a *api) setIcon(w http.ResponseWriter, r *http.Request) {
	stack := chi.URLParam(r, "stack")
	service := chi.URLParam(r, "service")
	var body struct {
		Icon string `json:"icon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	if err := a.db.SetServiceIcon(stack, service, strings.TrimSpace(body.Icon)); err != nil {
		a.logger.Error("set icon", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to persist icon")
		return
	}
	a.hub.NotifyChanged("prefs")
	w.WriteHeader(http.StatusNoContent)
}

// setName persists a custom display name for a service's card; empty clears
// it back to the automatic name.
func (a *api) setName(w http.ResponseWriter, r *http.Request) {
	stack := chi.URLParam(r, "stack")
	service := chi.URLParam(r, "service")
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	if err := a.db.SetServiceName(stack, service, strings.TrimSpace(body.Name)); err != nil {
		a.logger.Error("set name", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to persist name")
		return
	}
	a.hub.NotifyChanged("prefs")
	w.WriteHeader(http.StatusNoContent)
}

// setUrl persists a user's custom link URL for a service's card; empty clears
// it back to the automatic port-derived link.
func (a *api) setUrl(w http.ResponseWriter, r *http.Request) {
	stack := chi.URLParam(r, "stack")
	service := chi.URLParam(r, "service")
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	url := strings.TrimSpace(body.URL)
	if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		writeError(w, http.StatusBadRequest, "URL must start with http:// or https://")
		return
	}
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	if err := a.db.SetServiceURL(stack, service, url); err != nil {
		a.logger.Error("set url", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to persist link")
		return
	}
	a.hub.NotifyChanged("prefs")
	w.WriteHeader(http.StatusNoContent)
}

// urlOverrides returns a lookup over persisted custom link URLs.
func (a *api) urlOverrides() (func(stack, service string) (string, bool), error) {
	if a.db == nil {
		return nil, nil
	}
	m, err := a.db.ServiceURLOverrides()
	if err != nil {
		return nil, err
	}
	return func(stack, service string) (string, bool) {
		if svcs, ok := m[stack]; ok {
			if v, ok := svcs[service]; ok {
				return v, true
			}
		}
		return "", false
	}, nil
}

// nameOverrides returns a lookup over persisted custom display names.
func (a *api) nameOverrides() (func(stack, service string) (string, bool), error) {
	if a.db == nil {
		return nil, nil
	}
	m, err := a.db.ServiceNameOverrides()
	if err != nil {
		return nil, err
	}
	return func(stack, service string) (string, bool) {
		if svcs, ok := m[stack]; ok {
			if v, ok := svcs[service]; ok {
				return v, true
			}
		}
		return "", false
	}, nil
}

// icon serves an image slug's icon: cache → CDN → 404 (UI falls back to a
// letter avatar). Slugs are validated by the resolver.
func (a *api) icon(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	data, ct, ok := a.icons.Icon(r.Context(), slug)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

// remoteIcon proxies a user-set custom icon URL through HiveDock — fetched
// server-side (SSRF-guarded) and cached — so the browser only ever loads icons
// from 'self' and the CSP needs no external image origins (§4.5).
func (a *api) remoteIcon(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		http.NotFound(w, r)
		return
	}
	data, ct, ok := a.icons.RemoteIcon(r.Context(), rawURL)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

func (a *api) publicHost(r *http.Request) string {
	if a.cfg.PublicHost != "" {
		return a.cfg.PublicHost
	}
	return r.Host
}

// hiddenOverrides returns a lookup over persisted service visibility prefs.
func (a *api) hiddenOverrides() (func(stack, service string) (bool, bool), error) {
	if a.db == nil {
		return nil, nil
	}
	m, err := a.db.ServiceHiddenOverrides()
	if err != nil {
		return nil, err
	}
	return func(stack, service string) (bool, bool) {
		if svcs, ok := m[stack]; ok {
			if v, ok := svcs[service]; ok {
				return v, true
			}
		}
		return false, false
	}, nil
}

// iconOverrides returns a lookup over persisted custom-icon prefs.
func (a *api) iconOverrides() (func(stack, service string) (string, bool), error) {
	if a.db == nil {
		return nil, nil
	}
	m, err := a.db.ServiceIconOverrides()
	if err != nil {
		return nil, err
	}
	return func(stack, service string) (string, bool) {
		if svcs, ok := m[stack]; ok {
			if v, ok := svcs[service]; ok {
				return v, true
			}
		}
		return "", false
	}, nil
}
