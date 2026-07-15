package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// RemoteIcon fetches and caches a user-supplied custom icon URL server-side, so
// the browser only ever loads icons from HiveDock itself (CSP img-src 'self'
// data:, HARDENING.md §4.5). The fetch is SSRF-guarded at dial time: the actual
// connection IP must be public, which defeats DNS rebinding and redirect tricks
// aimed at the Docker host's internal network or the cloud metadata endpoint.
func (ir *IconResolver) RemoteIcon(ctx context.Context, rawURL string) (data []byte, contentType string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, "", false
	}

	key := "remote-" + hashKey(rawURL)
	if data, ct, ok := ir.readCacheExt(key); ok {
		return data, ct, true
	}

	body, ct, ok := safeFetchImage(ctx, rawURL)
	if !ok {
		return nil, "", false
	}
	ext := extForContentType(ct)
	if ext == "" {
		return nil, "", false // not a recognized image type
	}
	ir.writeCache(key, ext, body)
	return body, ct, true
}

// readCacheExt reads a cached remote icon (any supported image extension).
func (ir *IconResolver) readCacheExt(key string) ([]byte, string, bool) {
	for _, e := range imageExts {
		p := filepath.Join(ir.cacheDir, key+"."+e.ext)
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			return data, e.ct, true
		}
	}
	return nil, "", false
}

// imageExts is the set of image types the proxy will cache and serve.
var imageExts = []struct{ ext, ct string }{
	{"svg", "image/svg+xml"},
	{"png", "image/png"},
	{"jpg", "image/jpeg"},
	{"webp", "image/webp"},
	{"gif", "image/gif"},
	{"ico", "image/x-icon"},
}

func extForContentType(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	switch ct {
	case "image/svg+xml":
		return "svg"
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	case "image/x-icon", "image/vnd.microsoft.icon":
		return "ico"
	}
	return ""
}

func hashKey(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:16]) // 32 hex chars, matches slugSafe [a-z0-9-]
}

// safeFetchImage fetches url with an SSRF-guarded HTTP client (public IPs only,
// bounded redirects, size + type limits, no environment proxy).
func safeFetchImage(ctx context.Context, url string) ([]byte, string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", false
	}
	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", false
	}
	ct := resp.Header.Get("Content-Type")
	if extForContentType(ct) == "" {
		return nil, "", false // reject non-image responses up front
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MiB cap
	if err != nil || len(data) == 0 {
		return nil, "", false
	}
	return data, ct, true
}

// ssrfSafeClient dials only public addresses. dialGuard runs after DNS
// resolution on every dial (including redirect hops), so a hostname that
// resolves — or rebinds — to a private/loopback/link-local address is refused.
var ssrfSafeClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		Proxy: nil, // never route through an env proxy
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
			Control: dialGuard,
		}).DialContext,
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("refusing redirect to scheme %q", req.URL.Scheme)
		}
		return nil
	},
}

// dialGuard rejects a connection whose resolved IP is not a public address.
func dialGuard(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !isPublicIP(ip) {
		return fmt.Errorf("refusing to connect to non-public address %q", address)
	}
	return nil
}

// isPublicIP reports whether ip is a globally routable unicast address — not
// loopback, private (RFC1918 / ULA), link-local (incl. 169.254.169.254 cloud
// metadata), CGNAT, multicast, or unspecified.
func isPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		// 100.64.0.0/10 carrier-grade NAT — internal-ish, not for icon fetches.
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return false
		}
	}
	return true
}
