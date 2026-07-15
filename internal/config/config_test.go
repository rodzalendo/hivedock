package config

import (
	"net"
	"os"
	"testing"
)

func TestParseCIDRs(t *testing.T) {
	got := parseCIDRs(" 10.0.0.0/8 , bogus , 192.168.1.0/24 ,, ::1/128 ")
	if len(got) != 3 {
		t.Fatalf("parsed %d CIDRs, want 3 (invalid dropped): %v", len(got), got)
	}
	// The parsed nets must actually match their intended ranges.
	checks := []struct {
		ip   string
		want bool
	}{
		{"10.1.2.3", true},
		{"192.168.1.5", true},
		{"172.16.0.1", false},
	}
	for _, c := range checks {
		ip := net.ParseIP(c.ip)
		matched := false
		for _, n := range got {
			if n.Contains(ip) {
				matched = true
				break
			}
		}
		if matched != c.want {
			t.Errorf("%s matched=%v, want %v", c.ip, matched, c.want)
		}
	}

	if parseCIDRs("") != nil {
		t.Error("empty input should yield nil")
	}
}

func TestAuthDisabledRemoved(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")
	if present, truthy := AuthDisabledRemoved(); !present || !truthy {
		t.Errorf("AUTH_DISABLED=true → present=%v truthy=%v, want true true", present, truthy)
	}
	t.Setenv("AUTH_DISABLED", "false")
	if present, truthy := AuthDisabledRemoved(); !present || truthy {
		t.Errorf("AUTH_DISABLED=false → present=%v truthy=%v, want true false", present, truthy)
	}
	os.Unsetenv("AUTH_DISABLED")
	if present, _ := AuthDisabledRemoved(); present {
		t.Errorf("unset → present=%v, want false", present)
	}
}
