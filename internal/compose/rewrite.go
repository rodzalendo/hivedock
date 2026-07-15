package compose

import (
	"bytes"
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
	if old == "" {
		return nil, ErrNoImage
	}
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

	off, err := valueOffset(content, imgNode.Line, old)
	if err != nil {
		return nil, err
	}
	result := make([]byte, 0, len(content)-len(old)+len(newRef))
	result = append(result, content[:off]...)
	result = append(result, newRef...)
	result = append(result, content[off+len(old):]...)

	// §5.3: prove the write touched exactly the intended image scalar and nothing
	// else. Any deviation aborts — a broken paper trail stops the press.
	if err := verifyExactRewrite(content, result, off, old, newRef, service, imgNode.Line); err != nil {
		return nil, fmt.Errorf("refusing compose rewrite: %w", err)
	}
	return result, nil
}

// valueOffset returns the absolute byte offset of old's first occurrence on the
// given 1-based line. SplitAfter keeps line terminators, so the running sum is an
// exact byte offset (correct for both "\n" and "\r\n").
func valueOffset(content []byte, line int, old string) (int, error) {
	lines := strings.SplitAfter(string(content), "\n")
	if line < 1 || line > len(lines) {
		return 0, fmt.Errorf("image value line %d out of range", line)
	}
	off := 0
	for i := 0; i < line-1; i++ {
		off += len(lines[i])
	}
	at := strings.Index(lines[line-1], old)
	if at < 0 {
		return 0, fmt.Errorf("could not locate image value %q on line %d", old, line)
	}
	return off + at, nil
}

// verifyExactRewrite enforces the byte-exactness invariant (§5.3): result must be
// content with ONLY the bytes [off, off+len(old)) replaced by newRef, and a
// re-parse must confirm that the changed scalar is the target service's image
// (still on its original line) — not some other same-line match. Either check
// failing aborts the write.
func verifyExactRewrite(original, result []byte, off int, old, newRef, service string, line int) error {
	if off < 0 || off+len(old) > len(original) {
		return fmt.Errorf("image span [%d,%d) out of range", off, off+len(old))
	}
	if !bytes.Equal(original[off:off+len(old)], []byte(old)) {
		return fmt.Errorf("image span does not hold the expected value")
	}
	expected := make([]byte, 0, len(original)-len(old)+len(newRef))
	expected = append(expected, original[:off]...)
	expected = append(expected, newRef...)
	expected = append(expected, original[off+len(old):]...)
	if !bytes.Equal(result, expected) {
		return fmt.Errorf("rewrite changed bytes outside the image tag span")
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(result, &doc); err != nil {
		return fmt.Errorf("rewrite produced unparseable YAML: %w", err)
	}
	img, err := findImageNode(&doc, service)
	if err != nil {
		return fmt.Errorf("rewrite lost the service image: %w", err)
	}
	if img.Value != newRef {
		return fmt.Errorf("service image is %q after rewrite, want %q", img.Value, newRef)
	}
	if img.Line != line {
		return fmt.Errorf("service image moved from line %d to %d", line, img.Line)
	}
	return nil
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
