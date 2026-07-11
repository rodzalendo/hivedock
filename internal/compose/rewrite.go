package compose

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Errors from SetImageTag that the caller surfaces rather than treating as
// failures — some images must never be rewritten.
var (
	// ErrEnvManaged: the image tag is env-interpolated (image: x:${TAG}). The
	// tag lives in .env, not the compose file — surface it, never rewrite it.
	ErrEnvManaged = errors.New("image tag is env-interpolated (managed via .env)")
	// ErrDigestPinned: the image is pinned by @sha256 digest — a tag rewrite
	// would change its meaning; leave it alone.
	ErrDigestPinned = errors.New("image is pinned by digest")
	// ErrServiceNotFound: the named service isn't in the compose file.
	ErrServiceNotFound = errors.New("service not found in compose file")
	// ErrNoImage: the service has no image: key (e.g. build-only).
	ErrNoImage = errors.New("service has no image")
)

// SetImageTag rewrites a single service's image tag to newTag, changing ONLY
// that scalar and leaving every other byte — comments, quoting, anchors,
// indentation, line endings — untouched. It locates the value via a read-only
// YAML parse (for robust structural addressing) but edits the raw bytes on the
// value's line, never re-serializing the document (which would reflow it).
//
// It refuses env-interpolated (ErrEnvManaged) and digest-pinned (ErrDigestPinned)
// images: those are surfaced to the user, not silently rewritten (see PLAN risk
// register). Returns the original content unchanged when the tag already matches.
func SetImageTag(content []byte, service, newTag string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("parse compose: %w", err)
	}

	imgNode, err := findImageNode(&doc, service)
	if err != nil {
		return nil, err
	}
	old := imgNode.Value
	if strings.Contains(old, "${") {
		return nil, ErrEnvManaged
	}
	if strings.Contains(old, "@") {
		return nil, ErrDigestPinned
	}

	newRef := replaceTag(old, newTag)
	if newRef == old {
		return content, nil
	}
	return replaceOnLine(content, imgNode.Line, old, newRef)
}

// findImageNode walks services.<service>.image to the scalar value node.
func findImageNode(doc *yaml.Node, service string) (*yaml.Node, error) {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("empty compose document")
	}
	root := doc.Content[0]
	services := mapValue(root, "services")
	if services == nil || services.Kind != yaml.MappingNode {
		return nil, ErrServiceNotFound
	}
	svc := mapValue(services, service)
	if svc == nil {
		return nil, ErrServiceNotFound
	}
	image := mapValue(svc, "image")
	if image == nil || image.Kind != yaml.ScalarNode {
		return nil, ErrNoImage
	}
	return image, nil
}

// mapValue returns the value node for key in a mapping node, or nil.
func mapValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// replaceTag swaps the tag portion of an image reference (or appends one if the
// reference is untagged). Digest refs are handled by the caller (rejected).
func replaceTag(image, newTag string) string {
	if i := strings.LastIndex(image, ":"); i >= 0 && !strings.Contains(image[i+1:], "/") {
		return image[:i+1] + newTag
	}
	return image + ":" + newTag
}

// replaceOnLine replaces the first occurrence of old with newRef on the given
// 1-based line, leaving all other lines (and the line's quoting/comments) intact.
func replaceOnLine(content []byte, line int, old, newRef string) ([]byte, error) {
	lines := strings.Split(string(content), "\n")
	idx := line - 1
	if idx < 0 || idx >= len(lines) {
		return nil, fmt.Errorf("image value line %d out of range", line)
	}
	at := strings.Index(lines[idx], old)
	if at < 0 {
		return nil, fmt.Errorf("could not locate image value %q on line %d", old, line)
	}
	lines[idx] = lines[idx][:at] + newRef + lines[idx][at+len(old):]
	return []byte(strings.Join(lines, "\n")), nil
}
