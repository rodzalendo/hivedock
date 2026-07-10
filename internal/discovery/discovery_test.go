package discovery

import (
	"testing"

	"github.com/rogalinski/hivedock/internal/docker"
	"github.com/rogalinski/hivedock/internal/stacks"
)

func TestNormalizeImage(t *testing.T) {
	cases := map[string]string{
		"nginx":                              "nginx",
		"nginx:1.27-alpine":                  "nginx",
		"library/redis:7.2":                  "redis",
		"docker.io/library/postgres:16":      "postgres",
		"ghcr.io/immich-app/immich-server":   "immich-server",
		"lscr.io/linuxserver/jellyfin:10.10": "jellyfin",
		"linuxserver/radarr:arm64v8-latest":  "radarr",
		"traefik/whoami:v1.10.1":             "whoami",
		"registry.example.com:5000/app:1.0":  "app",
		"jellyfin/jellyfin@sha256:abcdef":    "jellyfin",
		"":                                   "",
	}
	for in, want := range cases {
		if got := normalizeImage(in); got != want {
			t.Errorf("normalizeImage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAutoHide(t *testing.T) {
	// Datastore image with a published port -> still hidden.
	if !autoHide(stacks.Service{Image: "postgres:16", Ports: []docker.Port{{Public: 5432, Private: 5432}}}) {
		t.Error("postgres should auto-hide even with a published port")
	}
	// No published ports -> hidden.
	if !autoHide(stacks.Service{Image: "some/worker", Ports: nil}) {
		t.Error("portless service should auto-hide")
	}
	// Published http port, not a datastore -> visible.
	if autoHide(stacks.Service{Image: "jellyfin/jellyfin", Ports: []docker.Port{{Public: 8096, Private: 8096}}}) {
		t.Error("published app should be visible")
	}
}

func TestHumanize(t *testing.T) {
	cases := map[string]string{
		"uptime-kuma": "Uptime Kuma",
		"whoami":      "Whoami",
		"my_app":      "My App",
	}
	for in, want := range cases {
		if got := humanize(in); got != want {
			t.Errorf("humanize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestURLHeuristicSinglePort(t *testing.T) {
	svc := stacks.Service{Ports: []docker.Port{
		{Public: 8096, Private: 8096, Type: "tcp"},
		{Public: 8096, Private: 8096, Type: "tcp"}, // IPv6 dup, should dedupe
	}}
	url, ports := urlHeuristic(svc, "192.168.1.10:5001")
	if url != "http://192.168.1.10:8096" {
		t.Errorf("url = %q", url)
	}
	if ports != nil {
		t.Errorf("single port should have no dropdown, got %+v", ports)
	}
}

func TestURLHeuristicMultiPortPrefersHTTP(t *testing.T) {
	svc := stacks.Service{Ports: []docker.Port{
		{Public: 8025, Private: 8025, Type: "tcp"}, // mailpit UI
		{Public: 1025, Private: 1025, Type: "tcp"}, // smtp
		{Public: 8080, Private: 80, Type: "tcp"},   // http -> preferred
	}}
	url, ports := urlHeuristic(svc, "host:5001")
	if url != "http://host:8080" {
		t.Errorf("primary url = %q, want the :80-container port mapping", url)
	}
	if len(ports) != 3 {
		t.Errorf("expected 3 port links, got %d", len(ports))
	}
}

func TestURLHeuristicHTTPS(t *testing.T) {
	svc := stacks.Service{Ports: []docker.Port{{Public: 8443, Private: 443, Type: "tcp"}}}
	url, _ := urlHeuristic(svc, "host:5001")
	if url != "https://host:8443" {
		t.Errorf("url = %q, want https", url)
	}
}

func TestResolvePriorityChainsAndLabels(t *testing.T) {
	all := []stacks.Stack{
		{
			Name: "jellyfin", Origin: stacks.OriginManaged, Services: []stacks.Service{
				{
					Name: "jellyfin", Image: "jellyfin/jellyfin:10.10", State: "running",
					Ports: []docker.Port{{Public: 8096, Private: 8096, Type: "tcp"}},
				},
			},
		},
		{
			Name: "app-with-db", Origin: stacks.OriginManaged, Services: []stacks.Service{
				{Name: "web", Image: "traefik/whoami", State: "running", Ports: []docker.Port{{Public: 8083, Private: 80}}},
				{Name: "db", Image: "postgres:16", State: "running", Ports: []docker.Port{{Public: 5432, Private: 5432}}},
			},
		},
		{
			Name: "labeled", Origin: stacks.OriginManaged, Services: []stacks.Service{
				{
					Name: "svc", Image: "nginx", State: "running",
					Ports:  []docker.Port{{Public: 8084, Private: 80}},
					Labels: map[string]string{"homepage.name": "My Service", "homepage.group": "Media"},
				},
			},
		},
	}

	entries := Resolve(all, Options{Host: "192.168.1.10:5001"})

	get := func(stack, service string) (Entry, bool) {
		for _, e := range entries {
			if e.Stack == stack && e.Service == service {
				return e, true
			}
		}
		return Entry{}, false
	}

	// Single-candidate managed stack -> name defaults to the humanized stack name.
	jf, _ := get("jellyfin", "jellyfin")
	if jf.Name != "Jellyfin" || jf.URL != "http://192.168.1.10:8096" || jf.Hidden {
		t.Errorf("jellyfin entry = %+v", jf)
	}
	if jf.IconSlug != "jellyfin" {
		t.Errorf("jellyfin iconSlug = %q", jf.IconSlug)
	}

	// db sidecar auto-hidden; web visible.
	db, _ := get("app-with-db", "db")
	if !db.Hidden {
		t.Error("postgres db should be auto-hidden")
	}
	web, _ := get("app-with-db", "web")
	if web.Hidden {
		t.Error("web app should be visible")
	}

	// homepage.* labels honored as fallback.
	lab, _ := get("labeled", "svc")
	if lab.Name != "My Service" || lab.Group != "Media" {
		t.Errorf("labeled entry = %+v", lab)
	}
}

func TestResolveUserOverrideBeatsAutoHide(t *testing.T) {
	all := []stacks.Stack{{
		Name: "app-with-db", Origin: stacks.OriginManaged, Services: []stacks.Service{
			{Name: "db", Image: "postgres:16", State: "running", Ports: []docker.Port{{Public: 5432, Private: 5432}}},
		},
	}}
	opts := Options{
		Host: "h:1",
		HiddenOverride: func(stack, service string) (bool, bool) {
			if stack == "app-with-db" && service == "db" {
				return false, true // user chose to unhide
			}
			return false, false
		},
	}
	entries := Resolve(all, opts)
	if len(entries) != 1 || entries[0].Hidden {
		t.Fatalf("user unhide should win over auto-hide: %+v", entries)
	}
}
