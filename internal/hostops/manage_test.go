package hostops

import (
	"testing"

	"github.com/rogalinski/hivedock/internal/stacks"
)

// TestHasContainersIncludesStopped is the regression guard for orphaned stacks:
// deleting a stopped stack used to skip `compose down` (it only checked for
// *running* services), leaving exited containers behind. With the directory gone
// those containers still carry their compose project label, so the scanner
// reclassifies them as an external stack — which is read-only and therefore
// impossible to delete from the UI.
func TestHasContainersIncludesStopped(t *testing.T) {
	cases := []struct {
		name  string
		state string
		want  bool
	}{
		{"running", "running", true},
		{"exited", "exited", true},
		{"created", "created", true},
		{"paused", "paused", true},
		{"no container at all", "absent", false},
	}
	for _, tc := range cases {
		st := stacks.Stack{Services: []stacks.Service{{Name: "app", State: tc.state}}}
		if got := hasContainers(st); got != tc.want {
			t.Errorf("hasContainers(state=%q) = %v, want %v", tc.state, got, tc.want)
		}
	}

	// A stack with no services declared has nothing to tear down.
	if hasContainers(stacks.Stack{}) {
		t.Error("hasContainers on an empty stack should be false")
	}
	// Mixed: one absent service must not mask a real container on another.
	mixed := stacks.Stack{Services: []stacks.Service{
		{Name: "a", State: "absent"},
		{Name: "b", State: "exited"},
	}}
	if !hasContainers(mixed) {
		t.Error("hasContainers should be true when any service has a container")
	}
}
