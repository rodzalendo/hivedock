package registry

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockRT is an http.RoundTripper backed by a function, for offline client tests.
type mockRT struct {
	fn func(*http.Request) *http.Response
	mu sync.Mutex
	n  int
}

func (m *mockRT) RoundTrip(r *http.Request) (*http.Response, error) {
	m.mu.Lock()
	m.n++
	m.mu.Unlock()
	return m.fn(r), nil
}

func mockResponse(status int, headers map[string]string, body string) *http.Response {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body))}
}

func testClient(rt http.RoundTripper) *Client {
	c := NewClient(&http.Client{Transport: rt})
	c.baseBackoff = time.Millisecond // keep retries fast in tests
	return c
}

func TestParseImageRef(t *testing.T) {
	cases := []struct {
		in       string
		registry string
		repo     string
		tag      string
		digest   string
	}{
		{"nginx", "registry-1.docker.io", "library/nginx", "", ""},
		{"nginx:1.25", "registry-1.docker.io", "library/nginx", "1.25", ""},
		{"traefik/whoami:v1.10.1", "registry-1.docker.io", "traefik/whoami", "v1.10.1", ""},
		{"ghcr.io/immich-app/immich-server:v1.100", "ghcr.io", "immich-app/immich-server", "v1.100", ""},
		{"lscr.io/linuxserver/sonarr:4.0.0", "ghcr.io", "linuxserver/sonarr", "4.0.0", ""},
		{"quay.io/prometheus/prometheus:v2.53.0", "quay.io", "prometheus/prometheus", "v2.53.0", ""},
		{"localhost:5000/app:dev", "localhost:5000", "app", "dev", ""},
		{"nginx@sha256:abc", "registry-1.docker.io", "library/nginx", "", "sha256:abc"},
	}
	for _, tc := range cases {
		got, err := ParseImageRef(tc.in)
		if err != nil {
			t.Errorf("ParseImageRef(%q) error: %v", tc.in, err)
			continue
		}
		if got.Registry != tc.registry || got.Repo != tc.repo || got.Tag != tc.tag || got.Digest != tc.digest {
			t.Errorf("ParseImageRef(%q) = %+v, want registry=%q repo=%q tag=%q digest=%q",
				tc.in, got, tc.registry, tc.repo, tc.tag, tc.digest)
		}
	}
}

func TestDigestTokenChallengeFlow(t *testing.T) {
	rt := &mockRT{fn: func(r *http.Request) *http.Response {
		switch {
		case strings.Contains(r.URL.Host, "auth.example"):
			// Token endpoint must receive the challenge's service+scope.
			if r.URL.Query().Get("scope") != "repository:library/nginx:pull" {
				t.Errorf("token request missing scope: %s", r.URL.String())
			}
			return mockResponse(200, map[string]string{"Content-Type": "application/json"}, `{"token":"abc123"}`)
		case strings.Contains(r.URL.Path, "/manifests/"):
			if r.Header.Get("Authorization") == "Bearer abc123" {
				return mockResponse(200, map[string]string{"Docker-Content-Digest": "sha256:deadbeef"}, "")
			}
			return mockResponse(401, map[string]string{
				"WWW-Authenticate": `Bearer realm="https://auth.example/token",service="registry.docker.io",scope="repository:library/nginx:pull"`,
			}, "")
		}
		return mockResponse(404, nil, "")
	}}
	c := testClient(rt)

	ref, _ := ParseImageRef("nginx:1.27")
	dig, err := c.Digest(context.Background(), ref)
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	if dig != "sha256:deadbeef" {
		t.Errorf("digest = %q, want sha256:deadbeef", dig)
	}
}

func TestTokenFetchUsesBasicAuthWhenConfigured(t *testing.T) {
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("bob:s3cret"))
	var gotAuth string
	rt := &mockRT{fn: func(r *http.Request) *http.Response {
		switch {
		case strings.Contains(r.URL.Host, "auth.example"):
			gotAuth = r.Header.Get("Authorization")
			return mockResponse(200, map[string]string{"Content-Type": "application/json"}, `{"token":"abc"}`)
		case strings.Contains(r.URL.Path, "/manifests/"):
			if r.Header.Get("Authorization") == "Bearer abc" {
				return mockResponse(200, map[string]string{"Docker-Content-Digest": "sha256:x"}, "")
			}
			return mockResponse(401, map[string]string{
				"WWW-Authenticate": `Bearer realm="https://auth.example/token",service="reg",scope="repository:app:pull"`,
			}, "")
		}
		return mockResponse(404, nil, "")
	}}
	c := testClient(rt)
	c.SetConfigResolver(func(host string) HostConfig {
		if host == "registry.example.com" {
			return HostConfig{Username: "bob", Password: "s3cret"}
		}
		return HostConfig{}
	})
	ref, _ := ParseImageRef("registry.example.com/app:1")
	if _, err := c.Digest(context.Background(), ref); err != nil {
		t.Fatalf("Digest: %v", err)
	}
	if gotAuth != want {
		t.Errorf("token request Authorization = %q, want Basic-auth for the configured registry", gotAuth)
	}
}

func TestClientForHostTLS(t *testing.T) {
	c := testClient(&mockRT{fn: func(*http.Request) *http.Response { return mockResponse(200, nil, "") }})
	if c.clientForHost("public.io") != c.http {
		t.Error("no resolver → default client expected")
	}
	c.SetConfigResolver(func(host string) HostConfig {
		if host == "self.signed" {
			return HostConfig{Insecure: true}
		}
		return HostConfig{}
	})
	if c.clientForHost("public.io") != c.http {
		t.Error("public host should reuse the default client")
	}
	hc := c.clientForHost("self.signed")
	if hc == c.http {
		t.Error("insecure host should get a dedicated client")
	}
	if c.clientForHost("self.signed") != hc {
		t.Error("per-host client should be cached")
	}
}

func TestTagsPagination(t *testing.T) {
	rt := &mockRT{fn: func(r *http.Request) *http.Response {
		if !strings.Contains(r.URL.Path, "/tags/list") {
			return mockResponse(404, nil, "")
		}
		if r.URL.Query().Get("last") == "" {
			return mockResponse(200, map[string]string{
				"Link": `</v2/library/nginx/tags/list?n=100&last=1.1>; rel="next"`,
			}, `{"tags":["1.0","1.1"]}`)
		}
		return mockResponse(200, nil, `{"tags":["1.2"]}`)
	}}
	c := testClient(rt)

	ref, _ := ParseImageRef("nginx")
	tags, err := c.Tags(context.Background(), ref)
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	want := []string{"1.0", "1.1", "1.2"}
	if strings.Join(tags, ",") != strings.Join(want, ",") {
		t.Errorf("tags = %v, want %v", tags, want)
	}
}

func TestBackoffOn429(t *testing.T) {
	var calls int32
	rt := &mockRT{fn: func(r *http.Request) *http.Response {
		if atomic.AddInt32(&calls, 1) == 1 {
			return mockResponse(429, map[string]string{"Retry-After": "0"}, "")
		}
		return mockResponse(200, map[string]string{"Docker-Content-Digest": "sha256:ok"}, "")
	}}
	c := testClient(rt)

	ref, _ := ParseImageRef("nginx:1.27")
	dig, err := c.Digest(context.Background(), ref)
	if err != nil {
		t.Fatalf("Digest after 429: %v", err)
	}
	if dig != "sha256:ok" {
		t.Errorf("digest = %q, want sha256:ok", dig)
	}
	if n := atomic.LoadInt32(&calls); n < 2 {
		t.Errorf("made %d calls, expected a retry after 429", n)
	}
}

func TestConcurrencyLimitPerHost(t *testing.T) {
	var inFlight, maxSeen int32
	rt := &mockRT{fn: func(r *http.Request) *http.Response {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxSeen)
			if cur <= old || atomic.CompareAndSwapInt32(&maxSeen, old, cur) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return mockResponse(200, map[string]string{"Docker-Content-Digest": "sha256:x"}, "")
	}}
	c := testClient(rt)
	ref, _ := ParseImageRef("nginx:1.27")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Digest(context.Background(), ref)
		}()
	}
	wg.Wait()

	if maxSeen > int32(c.perHost) {
		t.Errorf("max concurrent requests = %d, want <= %d", maxSeen, c.perHost)
	}
}

// TestLiveDockerHub exercises the real anonymous token flow against Docker Hub.
// Skipped by default; run with REGISTRY_LIVE=1 (needs network).
func TestLiveDockerHub(t *testing.T) {
	if os.Getenv("REGISTRY_LIVE") != "1" {
		t.Skip("set REGISTRY_LIVE=1 to run the live Docker Hub test")
	}
	c := NewClient(nil)
	ref, err := ParseImageRef("traefik/whoami:v1.10.1")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dig, err := c.Digest(ctx, ref)
	if err != nil {
		t.Fatalf("live Digest: %v", err)
	}
	if !strings.HasPrefix(dig, "sha256:") {
		t.Errorf("digest = %q, want sha256:…", dig)
	}
	t.Logf("whoami:v1.10.1 digest = %s", dig)

	tags, err := c.Tags(ctx, ref)
	if err != nil {
		t.Fatalf("live Tags: %v", err)
	}
	if len(tags) < 5 {
		t.Errorf("got %d tags, expected many", len(tags))
	}
	cand, diff, ok := Candidate("v1.10.0", tags)
	t.Logf("candidate for whoami v1.10.0 across %d real tags: %q (%s) ok=%v", len(tags), cand, diff, ok)
}
