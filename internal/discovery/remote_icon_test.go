package discovery

import (
	"context"
	"net"
	"testing"
)

func TestIsPublicIPBlocksInternalRanges(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", // loopback
		"10.0.0.5", "172.16.0.1", "192.168.1.1", // RFC1918
		"169.254.169.254",     // cloud metadata (link-local)
		"fe80::1",             // IPv6 link-local
		"fc00::1", "fd12::34", // IPv6 ULA
		"100.64.0.1", "100.127.255.255", // CGNAT
		"0.0.0.0", "::", // unspecified
		"224.0.0.1", "ff02::1", // multicast
	}
	for _, s := range blocked {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("bad test IP %q", s)
		}
		if isPublicIP(ip) {
			t.Errorf("isPublicIP(%s) = true, want false (internal/reserved)", s)
		}
	}

	public := []string{"1.1.1.1", "8.8.8.8", "140.82.121.3", "2606:4700:4700::1111"}
	for _, s := range public {
		if !isPublicIP(net.ParseIP(s)) {
			t.Errorf("isPublicIP(%s) = false, want true (public)", s)
		}
	}
}

func TestDialGuardRejectsPrivateAddress(t *testing.T) {
	if err := dialGuard("tcp", "169.254.169.254:80", nil); err == nil {
		t.Error("dialGuard allowed the cloud metadata address")
	}
	if err := dialGuard("tcp", "10.1.2.3:443", nil); err == nil {
		t.Error("dialGuard allowed an RFC1918 address")
	}
	if err := dialGuard("tcp", "1.1.1.1:443", nil); err != nil {
		t.Errorf("dialGuard rejected a public address: %v", err)
	}
}

func TestRemoteIconRejectsNonHTTPURLs(t *testing.T) {
	ir := NewIconResolver(t.TempDir(), nil)
	for _, u := range []string{
		"file:///etc/passwd",
		"ftp://example.com/x.png",
		"data:image/png;base64,AAAA",
		"not a url",
		"",
		"http://", // no host
	} {
		if _, _, ok := ir.RemoteIcon(context.Background(), u); ok {
			t.Errorf("RemoteIcon(%q) = ok, want rejected", u)
		}
	}
}

func TestExtForContentType(t *testing.T) {
	cases := map[string]string{
		"image/svg+xml":              "svg",
		"image/png":                  "png",
		"image/jpeg; charset=binary": "jpg",
		"IMAGE/PNG":                  "png",
		"image/x-icon":               "ico",
		"image/vnd.microsoft.icon":   "ico",
		"text/html":                  "",
		"application/octet-stream":   "",
	}
	for ct, want := range cases {
		if got := extForContentType(ct); got != want {
			t.Errorf("extForContentType(%q) = %q, want %q", ct, got, want)
		}
	}
}
