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
