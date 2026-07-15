package server

import (
	"net/http/httptest"
	"testing"
)

func TestCheckWSOrigin(t *testing.T) {
	a := &api{}
	a.cfg.PublicHost = "hive.lan:5001"

	cases := []struct {
		host, origin string
		want         bool
	}{
		{"box:5001", "", true},                          // no Origin (curl/native) → allowed
		{"box:5001", "http://box:5001", true},           // same origin
		{"box:5001", "https://box:5001", true},          // scheme-agnostic, host matches
		{"box:5001", "http://evil.example", false},      // foreign origin → rejected
		{"internal:5001", "http://hive.lan:5001", true}, // matches PUBLIC_HOST
		{"box:5001", "://bad", false},                   // unparseable → rejected
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "http://"+c.host+"/api/ws", nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if got := a.checkWSOrigin(r); got != c.want {
			t.Errorf("checkWSOrigin(host=%q origin=%q) = %v, want %v", c.host, c.origin, got, c.want)
		}
	}
}
