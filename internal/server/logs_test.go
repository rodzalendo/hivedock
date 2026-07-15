package server

import (
	"reflect"
	"testing"
)

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
		if got := sanitizeLogLine(in); got != want {
			t.Errorf("sanitizeLogLine(%q) = %q, want %q", in, got, want)
		}
	}
}
