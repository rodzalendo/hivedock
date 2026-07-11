package registry

import (
	"fmt"
	"strings"
)

// Ref is a parsed, normalized image reference addressed at a registry's v2 API.
type Ref struct {
	Registry string // v2 API host, e.g. "registry-1.docker.io", "ghcr.io", "quay.io"
	Repo     string // full repository path, e.g. "library/nginx", "linuxserver/sonarr"
	Tag      string // tag if present (empty when pinned only by digest)
	Digest   string // "sha256:…" if the reference was digest-pinned
}

// String renders the reference back for display/logging.
func (r Ref) String() string {
	s := r.Registry + "/" + r.Repo
	if r.Tag != "" {
		s += ":" + r.Tag
	}
	if r.Digest != "" {
		s += "@" + r.Digest
	}
	return s
}

// ParseImageRef normalizes a compose `image:` value into a Ref pointed at the
// right registry v2 endpoint. It mirrors Docker's defaulting rules: a bare name
// is Docker Hub `library/<name>` served from registry-1.docker.io; it also
// rewrites linuxserver's `lscr.io` mirror to its `ghcr.io` origin (per PLAN).
func ParseImageRef(image string) (Ref, error) {
	s := strings.TrimSpace(image)
	if s == "" {
		return Ref{}, fmt.Errorf("empty image reference")
	}

	// Split off an @digest, if any.
	var digest string
	if i := strings.Index(s, "@"); i >= 0 {
		digest = s[i+1:]
		s = s[:i]
	}

	// The first path segment is the registry only if it looks like a host
	// (contains '.' or ':' or is localhost); otherwise it's a Docker Hub repo.
	registry := "docker.io"
	remainder := s
	if i := strings.Index(s, "/"); i >= 0 {
		head := s[:i]
		if head == "localhost" || strings.ContainsAny(head, ".:") {
			registry = head
			remainder = s[i+1:]
		}
	}

	// A trailing :tag is the last ':' with no '/' after it (guards host:port).
	tag := ""
	if i := strings.LastIndex(remainder, ":"); i >= 0 && !strings.Contains(remainder[i+1:], "/") {
		tag = remainder[i+1:]
		remainder = remainder[:i]
	}
	repo := remainder
	if repo == "" {
		return Ref{}, fmt.Errorf("invalid image reference %q: no repository", image)
	}

	// linuxserver's lscr.io is a mirror of ghcr.io/linuxserver/*.
	if registry == "lscr.io" {
		registry = "ghcr.io"
	}

	apiHost := registry
	switch registry {
	case "docker.io":
		// Official images live under library/; the v2 API host is registry-1.
		if !strings.Contains(repo, "/") {
			repo = "library/" + repo
		}
		apiHost = "registry-1.docker.io"
	}

	return Ref{Registry: apiHost, Repo: repo, Tag: tag, Digest: digest}, nil
}
