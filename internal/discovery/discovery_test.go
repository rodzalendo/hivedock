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

func TestPrimaryServiceAndSidecars(t *testing.T) {
	port := func(p uint16) []docker.Port { return []docker.Port{{Public: p, Private: p, Type: "tcp"}} }
	all := []stacks.Stack{
		{
			// Prefix match: immich-server is primary (shortest prefixed name);
			// machine-learning becomes a sidecar; datastores stay hidden.
			Name: "immich", Origin: stacks.OriginManaged, Services: []stacks.Service{
				{Name: "immich-server", Image: "ghcr.io/immich-app/immich-server:v1.135.0", State: "running", Ports: port(2283)},
				{Name: "immich-machine-learning", Image: "ghcr.io/immich-app/immich-machine-learning:v1.135.0", State: "running", Ports: port(3003)},
				{Name: "redis", Image: "redis:6.2", State: "running", Ports: port(6379)},
			},
		},
		{
			// Exact match: qbittorrent primary; helpers become sidecars.
			Name: "qbittorrent", Origin: stacks.OriginManaged, Services: []stacks.Service{
				{Name: "qbittorrent", Image: "lscr.io/linuxserver/qbittorrent:5.0", State: "running", Ports: port(8080)},
				{Name: "mousehole", Image: "some/mousehole", State: "running", Ports: port(9999)},
			},
		},
		{
			// No service relates to the stack name -> no primary, no sidecars.
			Name: "media", Origin: stacks.OriginManaged, Services: []stacks.Service{
				{Name: "jellyfin", Image: "jellyfin/jellyfin", State: "running", Ports: port(8096)},
				{Name: "sonarr", Image: "linuxserver/sonarr", State: "running", Ports: port(8989)},
			},
		},
	}

	entries := Resolve(all, Options{Host: "h:1"})
	get := func(stack, service string) Entry {
		for _, e := range entries {
			if e.Stack == stack && e.Service == service {
				return e
			}
		}
		t.Fatalf("missing entry %s/%s", stack, service)
		return Entry{}
	}

	if e := get("immich", "immich-server"); e.Sidecar || e.Hidden {
		t.Errorf("immich-server should be the primary card: %+v", e)
	}
	if e := get("immich", "immich-machine-learning"); !e.Sidecar {
		t.Errorf("immich-machine-learning should be a sidecar: %+v", e)
	}
	// Hidden datastores stay hidden, not sidecar (hidden already rolls up).
	if e := get("immich", "redis"); !e.Hidden || e.Sidecar {
		t.Errorf("redis should be hidden, not sidecar: %+v", e)
	}
	if e := get("qbittorrent", "qbittorrent"); e.Sidecar {
		t.Errorf("qbittorrent should be primary: %+v", e)
	}
	if e := get("qbittorrent", "mousehole"); !e.Sidecar {
		t.Errorf("mousehole should be a sidecar: %+v", e)
	}
	for _, svc := range []string{"jellyfin", "sonarr"} {
		if e := get("media", svc); e.Sidecar {
			t.Errorf("media/%s should keep its own card: %+v", svc, e)
		}
	}
}

func TestPrimaryLabelWins(t *testing.T) {
	all := []stacks.Stack{{
		Name: "media", Origin: stacks.OriginManaged, Services: []stacks.Service{
			{Name: "jellyfin", Image: "jellyfin/jellyfin", State: "running",
				Ports:  []docker.Port{{Public: 8096, Private: 8096}},
				Labels: map[string]string{"hivedock.primary": "true"}},
			{Name: "sonarr", Image: "linuxserver/sonarr", State: "running",
				Ports: []docker.Port{{Public: 8989, Private: 8989}}},
		},
	}}
	entries := Resolve(all, Options{Host: "h:1"})
	for _, e := range entries {
		if e.Service == "jellyfin" && e.Sidecar {
			t.Errorf("labeled primary should not be a sidecar: %+v", e)
		}
		if e.Service == "sonarr" && !e.Sidecar {
			t.Errorf("sonarr should be a sidecar of the labeled primary: %+v", e)
		}
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
