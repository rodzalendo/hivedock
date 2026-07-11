package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// acceptManifests is the Accept header for manifest requests: multi-arch indexes
// first, then single manifests, in both OCI and Docker media types. We only need
// the digest from the HEAD response, so any of these is fine.
const acceptManifests = "application/vnd.oci.image.index.v1+json, " +
	"application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.oci.image.manifest.v1+json, " +
	"application/vnd.docker.distribution.manifest.v2+json"

// Client talks to Docker Registry v2 APIs with anonymous bearer-token auth. It
// caches per-repository tokens, limits concurrency per registry host, and backs
// off on 429/5xx (Docker Hub rate limits are real — see PLAN risk register).
type Client struct {
	http        *http.Client
	perHost     int           // max concurrent requests per registry host
	maxRetry    int           // retry attempts on 429/5xx/network error
	baseBackoff time.Duration // initial backoff (doubles each retry)

	mu     sync.Mutex
	tokens map[string]string        // "host|repo" -> bearer token
	sems   map[string]chan struct{} // host -> concurrency semaphore
}

// NewClient builds a Client. Pass nil to use a default http.Client; pass a
// custom one (e.g. with a mock Transport) for tests.
func NewClient(hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		http:        hc,
		perHost:     2, // per-registry concurrency limit (risk register)
		maxRetry:    3,
		baseBackoff: 500 * time.Millisecond,
		tokens:      map[string]string{},
		sems:        map[string]chan struct{}{},
	}
}

// Digest resolves the manifest digest for ref's tag via a HEAD request. This is
// the cheap path for mutable tags (`latest`): the digest changes when the image
// is republished. Uses "latest" if ref has no tag.
func (c *Client) Digest(ctx context.Context, ref Ref) (string, error) {
	reference := ref.Tag
	if ref.Digest != "" {
		reference = ref.Digest
	}
	if reference == "" {
		reference = "latest"
	}
	u := fmt.Sprintf("https://%s/v2/%s/manifests/%s", ref.Registry, ref.Repo, reference)
	resp, err := c.doAuthed(ctx, http.MethodHead, ref, u, acceptManifests)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("manifest HEAD %s: %s", ref, resp.Status)
	}
	d := resp.Header.Get("Docker-Content-Digest")
	if d == "" {
		return "", fmt.Errorf("manifest for %s has no Docker-Content-Digest", ref)
	}
	return d, nil
}

// Tags lists all tags for ref's repository, following v2 Link-header pagination.
func (c *Client) Tags(ctx context.Context, ref Ref) ([]string, error) {
	next := fmt.Sprintf("https://%s/v2/%s/tags/list?n=100", ref.Registry, ref.Repo)
	var all []string
	for next != "" {
		resp, err := c.doAuthed(ctx, http.MethodGet, ref, next, "application/json")
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("tags list %s: %s", ref.Repo, resp.Status)
		}
		var body struct {
			Tags []string `json:"tags"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode tags for %s: %w", ref.Repo, err)
		}
		link := resp.Header.Get("Link")
		resp.Body.Close()
		all = append(all, body.Tags...)
		next = nextPageURL(link, ref.Registry)
	}
	return all, nil
}

// doAuthed performs a request, transparently handling the v2 token challenge:
// it tries with any cached token, and on a 401 Bearer challenge fetches a fresh
// token, caches it, and retries once.
func (c *Client) doAuthed(ctx context.Context, method string, ref Ref, reqURL, accept string) (*http.Response, error) {
	scopeKey := ref.Registry + "|" + ref.Repo

	resp, err := c.attempt(ctx, method, ref.Registry, reqURL, accept, c.getToken(scopeKey))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	ch := parseChallenge(resp.Header.Get("WWW-Authenticate"))
	resp.Body.Close()
	if ch == nil {
		return nil, fmt.Errorf("registry %s returned 401 without a bearer challenge", ref.Registry)
	}
	token, err := c.fetchToken(ctx, ch)
	if err != nil {
		return nil, err
	}
	c.setToken(scopeKey, token)
	return c.attempt(ctx, method, ref.Registry, reqURL, accept, token)
}

// attempt sends one request (with per-host concurrency), retrying on 429/5xx and
// network errors with exponential backoff (honoring Retry-After on 429).
func (c *Client) attempt(ctx context.Context, method, host, reqURL, accept, token string) (*http.Response, error) {
	backoff := c.baseBackoff
	var lastErr error

	for i := 0; i <= c.maxRetry; i++ {
		release := c.acquire(host)
		req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
		if err != nil {
			release()
			return nil, err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := c.http.Do(req)
		release()

		switch {
		case err != nil:
			lastErr = err
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			wait := backoff
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, e := strconv.Atoi(strings.TrimSpace(ra)); e == nil {
					wait = time.Duration(secs) * time.Second
				}
			}
			lastErr = fmt.Errorf("registry %s: %s", host, resp.Status)
			resp.Body.Close()
			if i == c.maxRetry {
				return nil, lastErr
			}
			if err := sleep(ctx, wait); err != nil {
				return nil, err
			}
			backoff *= 2
			continue
		default:
			return resp, nil
		}

		// Network error path: back off and retry.
		if i == c.maxRetry {
			break
		}
		if err := sleep(ctx, backoff); err != nil {
			return nil, err
		}
		backoff *= 2
	}
	return nil, fmt.Errorf("request to %s failed after %d attempts: %w", host, c.maxRetry+1, lastErr)
}

// fetchToken performs the anonymous OAuth2 token exchange for a Bearer challenge.
func (c *Client) fetchToken(ctx context.Context, ch *challenge) (string, error) {
	u, err := url.Parse(ch.realm)
	if err != nil {
		return "", fmt.Errorf("bad token realm %q: %w", ch.realm, err)
	}
	q := u.Query()
	if ch.service != "" {
		q.Set("service", ch.service)
	}
	if ch.scope != "" {
		q.Set("scope", ch.scope)
	}
	u.RawQuery = q.Encode()

	resp, err := c.attempt(ctx, http.MethodGet, u.Host, u.String(), "application/json", "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint %s: %s", u.Host, resp.Status)
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}
	if body.Token != "" {
		return body.Token, nil
	}
	if body.AccessToken != "" {
		return body.AccessToken, nil
	}
	return "", fmt.Errorf("token endpoint %s returned no token", u.Host)
}

func (c *Client) getToken(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokens[key]
}

func (c *Client) setToken(key, token string) {
	c.mu.Lock()
	c.tokens[key] = token
	c.mu.Unlock()
}

// acquire blocks until a concurrency slot for host is free, returning a release.
func (c *Client) acquire(host string) func() {
	c.mu.Lock()
	sem, ok := c.sems[host]
	if !ok {
		sem = make(chan struct{}, c.perHost)
		c.sems[host] = sem
	}
	c.mu.Unlock()

	sem <- struct{}{}
	return func() { <-sem }
}

// sleep waits for d or until ctx is cancelled.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// challenge is a parsed WWW-Authenticate Bearer challenge.
type challenge struct {
	realm   string
	service string
	scope   string
}

// parseChallenge parses `Bearer realm="…",service="…",scope="…"`.
func parseChallenge(header string) *challenge {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return nil
	}
	ch := &challenge{}
	for _, part := range splitParams(header[len(prefix):]) {
		eq := strings.IndexByte(part, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(part[:eq])
		val := strings.Trim(strings.TrimSpace(part[eq+1:]), `"`)
		switch key {
		case "realm":
			ch.realm = val
		case "service":
			ch.service = val
		case "scope":
			ch.scope = val
		}
	}
	if ch.realm == "" {
		return nil
	}
	return ch
}

// splitParams splits challenge params on commas that are not inside quotes.
func splitParams(s string) []string {
	var out []string
	var cur strings.Builder
	inQuotes := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuotes = !inQuotes
			cur.WriteByte(s[i])
		case ',':
			if inQuotes {
				cur.WriteByte(s[i])
			} else {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(s[i])
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// nextPageURL extracts the `rel="next"` target from a v2 Link header and makes
// it absolute against the registry host. Returns "" when there is no next page.
func nextPageURL(link, host string) string {
	if link == "" {
		return ""
	}
	for _, part := range strings.Split(link, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		start := strings.IndexByte(part, '<')
		end := strings.IndexByte(part, '>')
		if start < 0 || end < 0 || end <= start {
			continue
		}
		target := part[start+1 : end]
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			return target
		}
		return "https://" + host + target
	}
	return ""
}
