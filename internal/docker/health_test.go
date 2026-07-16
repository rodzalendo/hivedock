package docker

import "testing"

func TestParseHealth(t *testing.T) {
	cases := map[string]string{
		"Up 5 minutes (healthy)":           "healthy",
		"Up 5 minutes (unhealthy)":         "unhealthy",
		"Up 10 seconds (health: starting)": "starting",
		"Up 3 hours":                       "",
		"Exited (0) 2 minutes ago":         "",
		"":                                 "",
	}
	for status, want := range cases {
		if got := parseHealth(status); got != want {
			t.Errorf("parseHealth(%q) = %q, want %q", status, got, want)
		}
	}
}
