package compose

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// FuzzSetImageTag is the §4.8 guarantee behind §5.3: over arbitrary inputs, a
// successful SetImageTag may only change bytes on the image's own line — every
// other byte (comments, other services, quoting, anchors, blank lines, line
// endings) is left untouched — and the result must still parse.
func FuzzSetImageTag(f *testing.F) {
	seeds := []struct{ content, service, tag string }{
		{"services:\n  web:\n    image: nginx:1.0\n", "web", "1.1"},
		{"services:\n  a:\n    image: redis:7  # cache\n  b:\n    image: busybox:1\n", "a", "8"},
		{"name: proj\nservices:\n  x:\n    image: 'ghcr.io/o/r:1.2.3'\n", "x", "1.2.4"},
		{"services:\n  w:\n    image: \"nginx:1.25-alpine\"\n    ports: [\"80:80\"]\n", "w", "1.27-alpine"},
		{"services:\r\n  w:\r\n    image: nginx:1\r\n", "w", "2"}, // CRLF
		{"services:\n  w:\n    image: lscr.io/linuxserver/qbittorrent:5.0.0\n", "w", "5.1.0"},
	}
	for _, s := range seeds {
		f.Add(s.content, s.service, s.tag)
	}

	f.Fuzz(func(t *testing.T, content, service, tag string) {
		out, err := SetImageTag([]byte(content), service, tag)
		if err != nil {
			return // a refusal (env-managed, digest, not found, …) is fine
		}
		outStr := string(out)
		if outStr == content {
			return // no-op rewrite
		}

		// The change is a single contiguous span confined to one line: the bytes
		// before the common prefix and after the common suffix are identical, and
		// neither side's differing middle may contain a newline.
		p := commonPrefixLen(content, outStr)
		s := commonSuffixLen(content, outStr, p)
		oldMid := content[p : len(content)-s]
		newMid := outStr[p : len(outStr)-s]
		if strings.ContainsAny(oldMid, "\r\n") || strings.ContainsAny(newMid, "\r\n") {
			t.Fatalf("rewrite spilled across lines:\n content=%q\n out=%q\n oldMid=%q newMid=%q",
				content, outStr, oldMid, newMid)
		}

		// The result must still be valid YAML.
		var doc yaml.Node
		if err := yaml.Unmarshal(out, &doc); err != nil {
			t.Fatalf("rewrite produced unparseable YAML: %v\n%q", err, outStr)
		}
	})
}

func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

// commonSuffixLen counts trailing bytes shared by a and b without overrunning
// the already-counted common prefix p on either string.
func commonSuffixLen(a, b string, p int) int {
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	return s
}
