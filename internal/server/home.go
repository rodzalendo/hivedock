package server

import (
	"encoding/json"
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

	entries := discovery.Resolve(stacksList, discovery.Options{
		Host:           a.publicHost(r),
		HiddenOverride: overrides,
		IconOverride:   iconOverrides,
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
