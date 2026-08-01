package hostops

import (
	"reflect"
	"testing"

	"github.com/rogalinski/hivedock/internal/stacks"
)

// Logs must follow containers in ANY state — a crash-looping container is the
// one whose output the user needs. Filtering to "running" here was the bug that
// left the pane empty for a restarting service.
func TestLogTargetsIgnoresState(t *testing.T) {
	st := stacks.Stack{Services: []stacks.Service{
		{Name: "app", ContainerID: "aaa", State: "running"},
		{Name: "crashloop", ContainerID: "bbb", State: "restarting"},
		{Name: "dead", ContainerID: "ccc", State: "exited"},
		{Name: "paused", ContainerID: "ddd", State: "paused"},
		{Name: "never-deployed", State: "absent"}, // no container: nothing to follow
	}}

	var got []string
	for _, svc := range logTargets(st) {
		got = append(got, svc.Name)
	}
	want := []string{"app", "crashloop", "dead", "paused"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("logTargets = %v, want %v", got, want)
	}
}

func TestLineWriterSplitsAcrossChunks(t *testing.T) {
	var got []string
	w := &lineWriter{emit: func(l string) { got = append(got, l) }}

	// A line split across two Write calls must emit once, joined.
	w.Write([]byte("hello "))
	w.Write([]byte("world\nsecond line\npar"))
	w.Write([]byte("tial"))

	want := []string{"hello world", "second line"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}

	// The buffered partial line is emitted on flush.
	w.flush()
	want = append(want, "partial")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after flush got %q, want %q", got, want)
	}
}

func TestLineWriterTrimsCR(t *testing.T) {
	var got []string
	w := &lineWriter{emit: func(l string) { got = append(got, l) }}
	w.Write([]byte("windows\r\nline\r\n"))
	if want := []string{"windows", "line"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSanitizeLogLine(t *testing.T) {
	cases := map[string]string{
		"plain text":                   "plain text",
		"keeps\ttabs":                  "keeps\ttabs",
		"\x1b[31mred\x1b[0m error":     "red error", // CSI/SGR stripped
		"\x1b[2J\x1b[Hcleared":         "cleared",   // cursor/screen ops
		"\x1b]0;evil title\x07visible": "visible",   // OSC + BEL terminator
		"\x1b]0;st\x1b\\after":         "after",     // OSC + ST terminator
		"bell\x07 and null\x00 gone":   "bell and null gone",
		"del\x7f done":                 "del done",
		"\x1bcreset":                   "reset",          // two-byte escape
		"unicode ✓ café":               "unicode ✓ café", // multibyte preserved
	}
	for in, want := range cases {
		if got := SanitizeLogLine(in); got != want {
			t.Errorf("SanitizeLogLine(%q) = %q, want %q", in, got, want)
		}
	}
}
