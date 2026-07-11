package server

import (
	"bytes"
	"context"
	"encoding/json"
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
	if a.db != nil {
		if c, err := a.db.ImageChecks(); err != nil {
			a.logger.Warn("updates: load cache", "err", err)
		} else {
			cache = c
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

// runUpdateCheck performs the check (off the request path), persists results,
// broadcasts updates:changed, and fires the webhook for newly-found updates. It
// always clears the in-flight guard.
func (a *api) runUpdateCheck(images []string) {
	defer a.checking.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Snapshot the prior state so we only webhook on *newly* discovered updates.
	var prior map[string]updates.Result
	if a.db != nil {
		prior, _ = a.db.ImageChecks()
	}

	a.logger.Info("update check started", "images", len(images))
	results := a.checker.CheckAll(ctx, images)

	if a.db != nil {
		if err := a.db.SaveImageChecks(results); err != nil {
			a.logger.Error("updates: save results", "err", err)
		}
	}

	updated := 0
	var fresh []updates.Result
	for _, r := range results {
		if !r.HasUpdate {
			continue
		}
		updated++
		p, existed := prior[r.Image]
		if !existed || !p.HasUpdate || p.Candidate != r.Candidate || p.LatestDigest != r.LatestDigest {
			fresh = append(fresh, r)
		}
	}
	a.logger.Info("update check finished", "images", len(images), "updates", updated, "new", len(fresh))
	a.hub.Publish(events.Message{Type: "updates:changed", Payload: map[string]int{"updates": updated}})

	if webhook := a.effectiveWebhookURL(); len(fresh) > 0 && webhook != "" {
		a.sendWebhook(webhook, fresh)
	}
}

// sendWebhook POSTs a JSON payload describing newly-found updates to the
// configured single webhook URL. Best-effort (logged, never blocks a check).
func (a *api) sendWebhook(webhookURL string, newUpdates []updates.Result) {
	type item struct {
		Image     string `json:"image"`
		Kind      string `json:"kind"`
		Current   string `json:"current,omitempty"`
		Candidate string `json:"candidate,omitempty"`
		Diff      string `json:"diff,omitempty"`
	}
	items := make([]item, 0, len(newUpdates))
	for _, r := range newUpdates {
		items = append(items, item{Image: r.Image, Kind: r.Kind, Current: r.Current, Candidate: r.Candidate, Diff: r.Diff})
	}
	payload := map[string]any{
		"event":   "updates_available",
		"time":    time.Now().UTC().Format(time.RFC3339),
		"count":   len(items),
		"updates": items,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		a.logger.Error("webhook: marshal", "err", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		a.logger.Error("webhook: build request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.logger.Warn("webhook: post failed", "err", err)
		return
	}
	defer resp.Body.Close()
	a.logger.Info("webhook sent", "count", len(items), "status", resp.StatusCode)
}

// startUpdateScheduler runs an initial check shortly after startup, then repeats
// every cfg.CheckInterval, until ctx is cancelled. A no-op when the interval is
// 0 (disabled) or there's no store.
func (a *api) startUpdateScheduler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		timer := time.NewTimer(30 * time.Second) // let the daemon/stacks settle first
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				a.backgroundCheck(ctx)
				timer.Reset(interval)
			}
		}
	}()
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
