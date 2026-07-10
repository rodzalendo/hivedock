package discovery

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// iconAliases corrects the common cases where the normalized image slug doesn't
// match the dashboard-icons name. Matching a manifest/alias table (not blind URL
// construction) is required — names are irregular (docs/ARCHITECTURE.md §3.4).
var iconAliases = map[string]string{
	"ubuntu":         "ubuntu-linux",
	"postgres":       "postgresql",
	"mongo":          "mongodb",
	"whoami":         "traefik",
	"pihole":         "pi-hole",
	"homeassistant":  "home-assistant",
	"linuxserver":    "linuxserver-io",
	"vaultwarden":    "vaultwarden",
	"adguardhome":    "adguard-home",
}

// FetchFunc retrieves an icon by URL. Returns the bytes, content type, and
// whether it was found. Injectable so the resolver is testable offline.
type FetchFunc func(ctx context.Context, url string) (data []byte, contentType string, ok bool)

// IconResolver serves icons for image slugs: local cache → dashboard-icons CDN →
// miss (404; the UI then renders a letter avatar). Successful fetches are cached
// under DATA_DIR/icons so later loads are local and work airgapped.
type IconResolver struct {
	cacheDir string
	fetch    FetchFunc

	mu       sync.Mutex
	negative map[string]time.Time // slugs recently known to miss (re-check after TTL)
}

const (
	cdnBase     = "https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons"
	negativeTTL = 6 * time.Hour
)

var slugSafe = regexp.MustCompile(`^[a-z0-9-]+$`)

// NewIconResolver creates a resolver caching under dataDir/icons. A nil fetch
// uses the default HTTP fetcher against the dashboard-icons CDN.
func NewIconResolver(dataDir string, fetch FetchFunc) *IconResolver {
	if fetch == nil {
		fetch = defaultFetch
	}
	return &IconResolver{
		cacheDir: filepath.Join(dataDir, "icons"),
		fetch:    fetch,
		negative: map[string]time.Time{},
	}
}

// Icon returns the icon bytes and content type for a slug, or ok=false on miss.
func (ir *IconResolver) Icon(ctx context.Context, slug string) (data []byte, contentType string, ok bool) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !slugSafe.MatchString(slug) {
		return nil, "", false
	}
	if alias, has := iconAliases[slug]; has {
		slug = alias
	}

	// 1. Local cache.
	if data, ct, ok := ir.readCache(slug); ok {
		return data, ct, true
	}

	// 2. Negative cache (avoid hammering the CDN for known misses).
	ir.mu.Lock()
	if until, miss := ir.negative[slug]; miss && time.Now().Before(until.Add(negativeTTL)) {
		ir.mu.Unlock()
		return nil, "", false
	}
	ir.mu.Unlock()

	// 3. CDN: try svg then png.
	for _, ext := range []struct{ ext, ct string }{{"svg", "image/svg+xml"}, {"png", "image/png"}} {
		url := cdnBase + "/" + ext.ext + "/" + slug + "." + ext.ext
		if body, _, found := ir.fetch(ctx, url); found && len(body) > 0 {
			ir.writeCache(slug, ext.ext, body)
			return body, ext.ct, true
		}
	}

	ir.mu.Lock()
	ir.negative[slug] = time.Now()
	ir.mu.Unlock()
	return nil, "", false
}

func (ir *IconResolver) readCache(slug string) ([]byte, string, bool) {
	for _, e := range []struct{ ext, ct string }{{"svg", "image/svg+xml"}, {"png", "image/png"}} {
		p := filepath.Join(ir.cacheDir, slug+"."+e.ext)
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			return data, e.ct, true
		}
	}
	return nil, "", false
}

func (ir *IconResolver) writeCache(slug, ext string, data []byte) {
	if err := os.MkdirAll(ir.cacheDir, 0o755); err != nil {
		return
	}
	tmp := filepath.Join(ir.cacheDir, slug+"."+ext+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, filepath.Join(ir.cacheDir, slug+"."+ext))
}

func defaultFetch(ctx context.Context, url string) ([]byte, string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MiB cap
	if err != nil {
		return nil, "", false
	}
	return data, resp.Header.Get("Content-Type"), true
}
