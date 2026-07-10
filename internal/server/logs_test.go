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
