// Package registry contains the update-detection machinery (Phase 4): the
// semver candidate engine and (later) the registry v2 client. The candidate
// engine is intentionally built and tested against a real-world corpus
// (testdata/candidates.yaml) before any network code — wrong update suggestions
// are the feature's trust-killer (see docs/PLAN.md risk register).
package registry

import (
	"regexp"
	"strconv"
	"strings"
)

// DiffType classifies the significance of a candidate update relative to the
// current tag, based on the first version component that differs.
type DiffType string

const (
	DiffMajor DiffType = "major"
	DiffMinor DiffType = "minor"
	DiffPatch DiffType = "patch"
	DiffNone  DiffType = "" // no update / not applicable
)

// archPrefixes are image-tag architecture prefixes we strip (and preserve) so
// they don't get mistaken for a version. Longest/most-specific first. These
// contain digits (arm64v8-, amd64-), which is exactly why a naive
// "first numeric run" parse fails and we strip them up front.
var archPrefixes = []string{
	"arm64v8-", "arm32v7-", "arm32v6-", "arm32v5-",
	"ppc64le-", "riscv64-", "x86_64-",
	"amd64-", "arm64-", "armhf-", "s390x-", "i386-", "386-",
}

// coreRe matches a leading dotted-numeric version core (e.g. "10.8.13", "16").
var coreRe = regexp.MustCompile(`^\d+(?:\.\d+)*`)

// parsedTag is a tag decomposed into a preserved prefix, its numeric version
// components, and a preserved suffix. Two tags are on the "same track" iff their
// prefix, suffix, and component count all match.
type parsedTag struct {
	prefix string
	parts  []int
	suffix string
}

// parseTag decomposes tag, or reports ok=false if it isn't a version-like tag
// (mutable tags like "latest", non-numeric tags, and signature/attestation tags
// all fail here — the digest path handles the mutable ones).
func parseTag(tag string) (parsedTag, bool) {
	if tag == "" {
		return parsedTag{}, false
	}
	// Signature / attestation / bare-digest pointer tags are never versions.
	if strings.HasPrefix(tag, "sha256-") || strings.HasSuffix(tag, ".sig") || strings.HasSuffix(tag, ".att") {
		return parsedTag{}, false
	}

	rest := tag
	var prefix strings.Builder

	// Strip a known architecture prefix.
	for _, ap := range archPrefixes {
		if strings.HasPrefix(rest, ap) {
			prefix.WriteString(ap)
			rest = rest[len(ap):]
			break
		}
	}
	// Strip a leading 'v' only when immediately followed by a digit (v1.2.3),
	// so "valkey" or "vault" aren't mistaken for versions.
	if len(rest) >= 2 && rest[0] == 'v' && rest[1] >= '0' && rest[1] <= '9' {
		prefix.WriteByte('v')
		rest = rest[1:]
	}

	core := coreRe.FindString(rest)
	if core == "" {
		return parsedTag{}, false
	}
	suffix := rest[len(core):]

	segs := strings.Split(core, ".")
	parts := make([]int, len(segs))
	for i, s := range segs {
		n, err := strconv.Atoi(s)
		if err != nil {
			return parsedTag{}, false
		}
		parts[i] = n
	}
	return parsedTag{prefix: prefix.String(), parts: parts, suffix: suffix}, true
}

// IsVersion reports whether tag looks like a comparable version (so the semver
// candidate path applies). Mutable tags like "latest"/"stable" return false and
// are handled by the digest path instead.
func IsVersion(tag string) bool {
	_, ok := parseTag(tag)
	return ok
}

// Candidate returns the best available update for a container currently on
// `current`: the greatest tag on the same track (same prefix, suffix, and
// component count) that is strictly newer. ok=false means no update — either
// `current` isn't version-like, or nothing newer exists on its track.
func Candidate(current string, available []string) (tag string, diff DiffType, ok bool) {
	cur, ok := parseTag(current)
	if !ok {
		return "", DiffNone, false
	}

	var bestTag string
	var bestParts []int
	for _, cand := range available {
		p, ok := parseTag(cand)
		if !ok {
			continue
		}
		// Same track: identical prefix, suffix, and component count.
		if p.prefix != cur.prefix || p.suffix != cur.suffix || len(p.parts) != len(cur.parts) {
			continue
		}
		if compareParts(p.parts, cur.parts) <= 0 {
			continue // not strictly newer
		}
		if bestParts == nil || compareParts(p.parts, bestParts) > 0 {
			bestParts = p.parts
			bestTag = cand
		}
	}
	if bestTag == "" {
		return "", DiffNone, false
	}
	return bestTag, classify(cur.parts, bestParts), true
}

// compareParts compares two equal-length component slices numerically.
func compareParts(a, b []int) int {
	for i := range a {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}

// classify names the update by the first differing component: index 0 = major,
// 1 = minor, ≥2 = patch. Slices are the same length (same-track guarantee).
func classify(from, to []int) DiffType {
	for i := range from {
		if from[i] != to[i] {
			switch i {
			case 0:
				return DiffMajor
			case 1:
				return DiffMinor
			default:
				return DiffPatch
			}
		}
	}
	return DiffNone
}
