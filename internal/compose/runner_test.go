package compose

import (
	"reflect"
	"testing"
)

func TestActionValid(t *testing.T) {
	for _, a := range []Action{ActionUp, ActionDown, ActionRestart, ActionPull, ActionStop, ActionRecreate, ActionUpdate} {
		if !a.Valid() {
			t.Errorf("%q should be valid", a)
		}
	}
	if Action("delete").Valid() {
		t.Error("unknown action reported valid")
	}
}

func TestSubcommandArgs(t *testing.T) {
	cases := map[Action][]string{
		ActionUp:      {"up", "-d"},
		ActionDown:    {"down"},
		ActionRestart: {"restart"},
		ActionPull:    {"pull"},
		ActionStop:    {"stop"},
		ActionUpdate:  {"up", "-d", "--pull", "always"},
	}
	for act, want := range cases {
		if got := subcommandArgs(act); !reflect.DeepEqual(got, want) {
			t.Errorf("subcommandArgs(%q) = %v, want %v", act, got, want)
		}
	}
}

func TestCommandArgsVolumes(t *testing.T) {
	base := Op{Stack: "media", ComposeFile: "/s/media/compose.yaml", ProjectDir: "/s/media"}

	down := base
	down.Action = ActionDown
	if got := commandArgs(down); contains(got, "-v") {
		t.Errorf("plain down must not remove volumes: %v", got)
	}

	downV := down
	downV.Volumes = true
	if got := commandArgs(downV); !contains(got, "-v") {
		t.Errorf("down with Volumes should pass -v: %v", got)
	}

	// Volumes is meaningless for every other action and must never leak into one.
	for _, a := range []Action{ActionUp, ActionRestart, ActionPull, ActionStop, ActionRecreate, ActionUpdate} {
		op := base
		op.Action = a
		op.Volumes = true
		if got := commandArgs(op); contains(got, "-v") {
			t.Errorf("%q with Volumes=true leaked -v: %v", a, got)
		}
	}
}

// TestDownVolumesNotAddressableAsAction guards the destructive path: actions are
// routed by URL segment, so a `down -v` action name would let any caller destroy
// volumes without the delete dialog's explicit opt-in.
func TestDownVolumesNotAddressableAsAction(t *testing.T) {
	for _, name := range []string{"down -v", "down-v", "down-volumes"} {
		if Action(name).Valid() {
			t.Errorf("%q must not be a valid action", name)
		}
	}
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func TestPerStackLock(t *testing.T) {
	r := NewRunner()

	release, ok := r.Start("app")
	if !ok {
		t.Fatal("first Start should acquire the lock")
	}
	if !r.Running("app") {
		t.Error("Running should be true while held")
	}

	// A second op on the same stack is rejected.
	if _, ok := r.Start("app"); ok {
		t.Error("second Start on a busy stack should fail")
	}

	// A different stack is independent.
	rel2, ok := r.Start("other")
	if !ok {
		t.Error("Start on a free stack should succeed")
	}
	rel2()

	release()
	if r.Running("app") {
		t.Error("Running should be false after release")
	}

	// After release the stack is acquirable again; double-release is a no-op.
	release()
	if _, ok := r.Start("app"); !ok {
		t.Error("Start should succeed after release")
	}
}

func TestLineEmitter(t *testing.T) {
	var lines []string
	w := &lineEmitter{emit: func(s string) { lines = append(lines, s) }}

	// Split across writes; \r trimmed; partial line held until flush.
	w.Write([]byte("Pulling image\nCreat"))
	w.Write([]byte("ing container\r\ndone (no newline)"))
	if want := []string{"Pulling image", "Creating container"}; !reflect.DeepEqual(lines, want) {
		t.Fatalf("mid-stream lines = %v, want %v", lines, want)
	}
	w.flush()
	if want := []string{"Pulling image", "Creating container", "done (no newline)"}; !reflect.DeepEqual(lines, want) {
		t.Fatalf("after flush = %v, want %v", lines, want)
	}
}
