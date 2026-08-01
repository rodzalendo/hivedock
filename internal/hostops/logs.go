package hostops

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
)

const logTailDefault = 200

// logRescanInterval is how often an open log stream re-resolves a stack's
// containers, to pick up the new IDs a deploy/recreate produces.
const logRescanInterval = 2 * time.Second

// streamOneContainer pipes one container's logs to onLine until ctx ends. It
// demuxes Docker's stdout/stderr framing (or reads a raw TTY stream) and
// sanitizes every line. Shared by the local and (via the agent) remote log paths.
func (b *LocalBackend) streamOneContainer(ctx context.Context, service, containerID string, tail int, onLine func(LogLine)) error {
	rc, tty, err := b.docker.ContainerLogs(ctx, containerID, tail, true)
	if err != nil {
		return err
	}
	defer rc.Close()

	// Cancelling the context must unblock the blocking read below.
	go func() {
		<-ctx.Done()
		_ = rc.Close()
	}()

	emit := func(stream, line string) {
		onLine(LogLine{Service: service, Stream: stream, Line: SanitizeLogLine(line)})
	}
	if tty {
		scanLines(rc, func(line string) { emit("stdout", line) })
		return nil
	}
	stdoutW := &lineWriter{emit: func(l string) { emit("stdout", l) }}
	stderrW := &lineWriter{emit: func(l string) { emit("stderr", l) }}
	_, _ = stdcopy.StdCopy(stdoutW, stderrW, rc)
	stdoutW.flush()
	stderrW.flush()
	return nil
}

// SanitizeLogLine strips terminal escape sequences and stray control bytes from a
// container log line. Container output is attacker-controlled the moment any
// container on the host is compromised; without this, escape sequences (cursor
// moves, OSC title/clipboard injection, SGR color) would pass straight through to
// the browser. We keep printable text and tabs, nothing else.
func SanitizeLogLine(s string) string {
	if strings.IndexFunc(s, func(r rune) bool {
		return r == 0x1b || (r < 0x20 && r != '\t') || r == 0x7f
	}) < 0 {
		return s
	}
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == 0x1b { // ESC — start of an escape sequence
			if i+1 >= len(runes) {
				break
			}
			switch runes[i+1] {
			case '[': // CSI: params until a final byte in 0x40–0x7E
				i += 2
				for i < len(runes) && !(runes[i] >= 0x40 && runes[i] <= 0x7e) {
					i++
				}
			case ']': // OSC: until BEL or ST (ESC \)
				i += 2
				for i < len(runes) {
					if runes[i] == 0x07 {
						break
					}
					if runes[i] == 0x1b && i+1 < len(runes) && runes[i+1] == '\\' {
						i++
						break
					}
					i++
				}
			default: // two-byte escape (e.g. ESC c): drop both bytes
				i++
			}
			continue
		}
		if (r < 0x20 && r != '\t') || r == 0x7f {
			continue // drop stray control bytes, keep tab
		}
		b.WriteRune(r)
	}
	return b.String()
}

// scanLines reads newline-delimited text and calls fn per line.
func scanLines(r io.Reader, fn func(string)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		fn(sc.Text())
	}
}

// lineWriter accumulates bytes and emits complete lines (splitting on '\n').
// stdcopy writes arbitrary chunks, so we buffer partial lines across writes.
type lineWriter struct {
	emit func(string)
	buf  []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := w.buf[:i]
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		w.emit(string(line))
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) > 1024*1024 {
		w.emit(string(w.buf))
		w.buf = w.buf[:0]
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		w.emit(string(w.buf))
		w.buf = w.buf[:0]
	}
}
