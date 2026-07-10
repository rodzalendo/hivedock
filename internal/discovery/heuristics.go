package discovery

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/rogalinski/hivedock/internal/stacks"
)

// firstLabel returns the first non-empty label value among keys.
func firstLabel(labels map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := labels[k]; ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// boolLabel parses the first present label among keys as a bool.
func boolLabel(labels map[string]string, keys ...string) (val, set bool) {
	for _, k := range keys {
		if v, ok := labels[k]; ok {
			if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
				return b, true
			}
		}
	}
	return false, false
}

// httpsContainerPorts are container-side ports that imply TLS.
var httpsContainerPorts = map[uint16]bool{443: true, 8443: true}

// preferredHTTPPorts orders which published port becomes a card's primary link.
var preferredHTTPPorts = []uint16{80, 8080, 3000, 8000, 8096, 443, 8443}

// urlHeuristic builds a primary URL and a full port list from a service's
// published ports (docs/ARCHITECTURE.md §3.3). host is where the browser
// reaches Hivedock; if empty, links are relative-hostless and omitted.
func urlHeuristic(svc stacks.Service, host string) (string, []PortLink) {
	type pub struct {
		public  uint16
		private uint16
		typ     string
	}
	var published []pub
	seen := map[uint16]bool{}
	for _, p := range svc.Ports {
		if p.Public == 0 || seen[p.Public] {
			continue // only published (host-mapped) ports; dedupe IPv4/IPv6
		}
		seen[p.Public] = true
		published = append(published, pub{p.Public, p.Private, p.Type})
	}
	if len(published) == 0 || host == "" {
		return "", nil
	}

	hostOnly := hostWithoutPort(host)

	links := make([]PortLink, 0, len(published))
	for _, p := range published {
		scheme := "http"
		if httpsContainerPorts[p.private] || httpsContainerPorts[p.public] {
			scheme = "https"
		}
		links = append(links, PortLink{
			Label: fmt.Sprintf("%d/%s", p.public, p.typ),
			URL:   fmt.Sprintf("%s://%s:%d", scheme, hostOnly, p.public),
		})
	}

	// Choose the primary link: a preferred http port (by container port), else
	// the lowest published port.
	primaryIdx := 0
	bestRank := len(preferredHTTPPorts)
	lowest := published[0].public
	for i, p := range published {
		for rank, pref := range preferredHTTPPorts {
			if p.private == pref && rank < bestRank {
				bestRank, primaryIdx = rank, i
			}
		}
		if p.public < lowest {
			lowest = p.public
		}
	}
	if bestRank == len(preferredHTTPPorts) {
		// No preferred port matched; pick the lowest published port.
		for i, p := range published {
			if p.public == lowest {
				primaryIdx = i
				break
			}
		}
	}

	// Single port: no dropdown needed.
	if len(links) == 1 {
		return links[0].URL, nil
	}
	sort.Slice(links, func(i, j int) bool { return links[i].Label < links[j].Label })
	return links[primaryIdx].URL, links
}

func hostWithoutPort(host string) string {
	// host may be "1.2.3.4:5001" or "example.com:5001" or bare.
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host[i:], "]") {
		// Avoid trimming inside an IPv6 literal like [::1]:5001.
		if !strings.Contains(host[:i], "]") || strings.HasPrefix(host, "[") {
			return strings.Trim(host[:i], "[]")
		}
	}
	return strings.Trim(host, "[]")
}

// datastoreImages are backend images auto-hidden even if they publish a port.
var datastoreImages = map[string]bool{
	"postgres": true, "postgresql": true, "mysql": true, "mariadb": true,
	"redis": true, "valkey": true, "keydb": true, "mongo": true, "mongodb": true,
	"rabbitmq": true, "memcached": true, "nats": true, "etcd": true,
	"clickhouse": true, "influxdb": true, "cockroachdb": true, "cassandra": true,
	"elasticsearch": true, "meilisearch": true, "qdrant": true,
}

// autoHide reports whether a service is infrastructure, not a user destination
// (docs/ARCHITECTURE.md §3.5): no published ports, or a known datastore image.
func autoHide(svc stacks.Service) bool {
	if datastoreImages[normalizeImage(svc.Image)] {
		return true
	}
	for _, p := range svc.Ports {
		if p.Public != 0 {
			return false // publishes a host port → user-facing
		}
	}
	return true
}

// humanize turns a service/stack name into a display label:
// "uptime-kuma" -> "Uptime Kuma".
func humanize(s string) string {
	s = strings.NewReplacer("-", " ", "_", " ", ".", " ").Replace(s)
	fields := strings.Fields(s)
	for i, f := range fields {
		fields[i] = strings.ToUpper(f[:1]) + f[1:]
	}
	return strings.Join(fields, " ")
}

// normalizeImage reduces an image reference to a lowercase kebab slug suitable
// for icon matching and datastore detection: strip registry, org, tag/digest,
// and common arch prefixes (docs/ARCHITECTURE.md §3.4).
func normalizeImage(image string) string {
	if image == "" {
		return ""
	}
	ref := image
	// Drop digest, then tag.
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndex(ref, ":"); i >= 0 && !strings.Contains(ref[i:], "/") {
		ref = ref[:i]
	}
	// Split path; drop a leading registry host (contains "." or ":" or is
	// "localhost"), keep the final path element as the image name.
	parts := strings.Split(ref, "/")
	if len(parts) > 1 && (strings.ContainsAny(parts[0], ".:") || parts[0] == "localhost") {
		parts = parts[1:]
	}
	name := parts[len(parts)-1]
	// Strip common linuxserver-style arch prefixes.
	for _, prefix := range []string{"arm64v8-", "arm32v7-", "amd64-", "arm64-", "amd-"} {
		name = strings.TrimPrefix(name, prefix)
	}
	name = strings.ToLower(name)
	// Kebab-case: replace anything not [a-z0-9] runs with a single '-'.
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
