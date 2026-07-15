package server

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/rogalinski/hivedock/internal/events"
	"github.com/rogalinski/hivedock/internal/stacks"
	"github.com/rogalinski/hivedock/internal/updates"
)

// usage names a place an image is used.
type usage struct {
	Stack   string `json:"stack"`
	Service string `json:"service"`
}

// updateEntry is one image's update status plus where it's used. The UI groups
// by image (a shared image used by several stacks is one row with usedBy).
type updateEntry struct {
	Image         string  `json:"image"`
	Kind          string  `json:"kind"` // includes "unchecked" until first check
	HasUpdate     bool    `json:"hasUpdate"`
	Current       string  `json:"current,omitempty"`
	Candidate     string  `json:"candidate,omitempty"`
	Diff          string  `json:"diff,omitempty"`
	CurrentDigest string  `json:"currentDigest,omitempty"`
	LatestDigest  string  `json:"latestDigest,omitempty"`
	Source        string  `json:"source,omitempty"`
	Error         string  `json:"error,omitempty"`
	CheckedAt     string  `json:"checkedAt,omitempty"`
	Ignored       bool    `json:"ignored,omitempty"` // user chose to keep the pinned version
	UsedBy        []usage `json:"usedBy"`
}

// listUpdates joins the images used by managed stacks with the cached check
// results. Images never checked yet appear with kind "unchecked".
func (a *api) listUpdates(w http.ResponseWriter, r *http.Request) {
	stackList, err := a.stacks.List(r.Context())
	if err != nil {
		a.logger.Error("updates: list stacks", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list stacks: "+err.Error())
		return
	}

	cache := map[string]updates.Result{}
	ignored := map[string]bool{}
	if a.db != nil {
		if c, err := a.db.ImageChecks(); err != nil {
			a.logger.Warn("updates: load cache", "err", err)
		} else {
			cache = c
		}
		if ig, err := a.db.IgnoredImages(); err != nil {
			a.logger.Warn("updates: load ignores", "err", err)
		} else {
			ignored = ig
		}
	}

	byImage := map[string]*updateEntry{}
	var order []string
	for _, st := range stackList {
		if st.Origin != stacks.OriginManaged {
			continue // only managed stacks are actionable
		}
		for _, svc := range st.Services {
			if svc.Image == "" {
				continue
			}
			e, ok := byImage[svc.Image]
			if !ok {
				e = &updateEntry{Image: svc.Image, Kind: "unchecked", UsedBy: []usage{}}
				if isEnvTemplated(svc.Image) {
					// Tag is an unresolved env var (e.g. ${IMMICH_VERSION}); we
					// can't check it without env substitution. Not an error.
					e.Kind = "unsupported"
				} else if res, cached := cache[svc.Image]; cached {
					e.Kind = res.Kind
					e.HasUpdate = res.HasUpdate
					e.Current = res.Current
					e.Candidate = res.Candidate
					e.Diff = res.Diff
					e.CurrentDigest = res.CurrentDigest
					e.LatestDigest = res.LatestDigest
					e.Source = res.Source
					e.Error = res.Error
					if !res.CheckedAt.IsZero() {
						e.CheckedAt = res.CheckedAt.UTC().Format(time.RFC3339)
					}
				}
				e.Ignored = ignored[svc.Image]
				byImage[svc.Image] = e
				order = append(order, svc.Image)
			}
			e.UsedBy = append(e.UsedBy, usage{Stack: st.Name, Service: svc.Name})
		}
	}

	sort.Strings(order)
	entries := make([]updateEntry, 0, len(order))
	for _, img := range order {
		entries = append(entries, *byImage[img])
	}
	writeJSON(w, http.StatusOK, entries)
}

// checkUpdates kicks off a registry check for every image used by managed
// stacks, runs it in the background, caches the results, and notifies clients.
// 409 if a check is already running.
func (a *api) checkUpdates(w http.ResponseWriter, r *http.Request) {
	if !a.checking.CompareAndSwap(false, true) {
		writeError(w, http.StatusConflict, "an update check is already running")
		return
	}

	stackList, err := a.stacks.List(r.Context())
	if err != nil {
		a.checking.Store(false)
		a.logger.Error("updates: list stacks", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list stacks: "+err.Error())
		return
	}
	images := managedImages(stackList)

	go a.runUpdateCheck(images)
	writeJSON(w, http.StatusAccepted, map[string]int{"images": len(images)})
}

// setIgnore records or clears a user's decision to ignore updates for a
// specific image reference (they've deliberately pinned that version). Ignored
// images are excluded from "Update all" and shown in their own UI section.
func (a *api) setIgnore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Image   string `json:"image"`
		Ignored bool   `json:"ignored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Image) == "" {
		writeError(w, http.StatusBadRequest, "invalid body: image is required")
		return
	}
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	if err := a.db.SetImageIgnored(strings.TrimSpace(body.Image), body.Ignored); err != nil {
		a.logger.Error("set ignore", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to persist ignore")
		return
	}
	a.hub.NotifyChanged("updates:ignore")
	w.WriteHeader(http.StatusNoContent)
}

// runUpdateCheck performs the check (off the request path), persists results,
// and broadcasts updates:changed. It always clears the in-flight guard.
func (a *api) runUpdateCheck(images []string) {
	defer a.checking.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	a.logger.Info("update check started", "images", len(images))
	results := a.checker.CheckAll(ctx, images)

	if a.db != nil {
		if err := a.db.SaveImageChecks(results); err != nil {
			a.logger.Error("updates: save results", "err", err)
		}
	}

	updated := 0
	for _, r := range results {
		if r.HasUpdate {
			updated++
		}
	}
	a.logger.Info("update check finished", "images", len(images), "updates", updated)
	a.hub.Publish(events.Message{Type: "updates:changed", Payload: map[string]int{"updates": updated}})
}

// startUpdateScheduler runs periodic update checks. The cadence is re-read
// from settings every minute (effectiveCheckInterval), so changing it in the
// UI — including turning it on or off — applies without a restart.
func (a *api) startUpdateScheduler(ctx context.Context) {
	go func() {
		// Let the daemon/stacks settle, then treat startup as "due now" when
		// checks are enabled.
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
		var lastRun time.Time
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			if iv := a.effectiveCheckInterval(); iv > 0 && time.Since(lastRun) >= iv {
				// Jitter the actual start across a slice of the window so sweeps
				// don't all hit registries at the same instant (§6.1). Per-host
				// concurrency + 429 backoff (registry client) handle the rest.
				if d := checkJitter(iv); d > 0 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(d):
					}
				}
				a.backgroundCheck(ctx)
				lastRun = time.Now()
			}
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
}

// checkJitter returns a random delay up to the smaller of interval/10 and 60s —
// enough to spread scheduled sweeps without meaningfully delaying them.
func checkJitter(interval time.Duration) time.Duration {
	max := interval / 10
	if max > time.Minute {
		max = time.Minute
	}
	if max <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(max)))
}

// backgroundCheck runs a scheduled check over managed-stack images (skipped if a
// check is already running).
func (a *api) backgroundCheck(ctx context.Context) {
	if !a.checking.CompareAndSwap(false, true) {
		return
	}
	stackList, err := a.stacks.List(ctx)
	if err != nil {
		a.checking.Store(false)
		a.logger.Warn("scheduled update check: list stacks", "err", err)
		return
	}
	a.runUpdateCheck(managedImages(stackList)) // clears the guard via defer
}

// isEnvTemplated reports whether an image reference still contains an
// unresolved environment variable (e.g. redis:${REDIS_TAG}). Such images can't
// be registry-checked without env substitution, so they're skipped, not errored.
func isEnvTemplated(image string) bool {
	return strings.Contains(image, "${") || strings.Contains(image, "$(")
}

// managedImages returns the distinct images across managed stacks.
func managedImages(stackList []stacks.Stack) []string {
	seen := map[string]bool{}
	var out []string
	for _, st := range stackList {
		if st.Origin != stacks.OriginManaged {
			continue
		}
		for _, svc := range st.Services {
			if svc.Image == "" || seen[svc.Image] || isEnvTemplated(svc.Image) {
				continue
			}
			seen[svc.Image] = true
			out = append(out, svc.Image)
		}
	}
	return out
}
