package server

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"
	"sync"

	"github.com/docker/docker/pkg/stdcopy"

	"github.com/rogalinski/hivedock/internal/events"
)

const logTailDefault = 200

// logLine is one streamed log line, tagged with its service and stream.
type logLine struct {
	Stack   string `json:"stack"`
	Service string `json:"service"`
	Stream  string `json:"stream"` // stdout | stderr
	Line    string `json:"line"`
}

// startLogs begins streaming `docker logs --follow` for every running container
// in a stack. Lines are pushed to the client as logs:line messages, tagged by
// service so the UI can filter. Re-subscribing to the same stack restarts it.
func (s *wsSession) startLogs(ctx context.Context, stackName string, tail int) {
	if stackName == "" {
		return
	}
	if s.api.docker == nil {
		s.send(events.Message{Type: "logs:error", Payload: map[string]string{
			"stack": stackName, "message": "docker daemon unavailable",
		}})
		return
	}
	if tail <= 0 {
		tail = logTailDefault
	}

	stack, ok, err := s.api.stacks.Get(ctx, stackName)
	if err != nil || !ok {
		s.send(events.Message{Type: "logs:error", Payload: map[string]string{
			"stack": stackName, "message": "stack not found",
		}})
		return
	}

	// Cancel any existing streams for this stack before starting fresh.
	s.stopLogs(stackName)

	streamCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		return
	}
	s.streams[stackName] = cancel
	s.mu.Unlock()

	var started int
	var wg sync.WaitGroup
	for _, svc := range stack.Services {
		if svc.ContainerID == "" || svc.State != "running" {
			continue
		}
		started++
		wg.Add(1)
		go func(service, containerID string) {
			defer wg.Done()
			s.streamContainer(streamCtx, stackName, service, containerID, tail)
		}(svc.Name, svc.ContainerID)
	}

	if started == 0 {
		s.send(events.Message{Type: "logs:error", Payload: map[string]string{
			"stack": stackName, "message": "no running containers to stream",
		}})
		s.stopLogs(stackName)
		return
	}

	// When all container streams end (containers stopped / follow ended), tell
	// the client so it can update the follow indicator.
	go func() {
		wg.Wait()
		s.send(events.Message{Type: "logs:end", Payload: map[string]string{"stack": stackName}})
	}()
}

func (s *wsSession) stopLogs(stackName string) {
	s.mu.Lock()
	if cancel, ok := s.streams[stackName]; ok {
		cancel()
		delete(s.streams, stackName)
	}
	s.mu.Unlock()
}

// streamContainer pipes one container's logs to the client until ctx ends.
func (s *wsSession) streamContainer(ctx context.Context, stack, service, containerID string, tail int) {
	rc, tty, err := s.api.docker.ContainerLogs(ctx, containerID, tail, true)
	if err != nil {
		if ctx.Err() == nil {
			s.send(events.Message{Type: "logs:error", Payload: map[string]string{
				"stack": stack, "service": service, "message": err.Error(),
			}})
		}
		return
	}
	defer rc.Close()

	// Cancelling the context must unblock the blocking read below.
	go func() {
		<-ctx.Done()
		_ = rc.Close()
	}()

	emit := func(stream, line string) {
		s.send(events.Message{Type: "logs:line", Payload: logLine{
			Stack: stack, Service: service, Stream: stream, Line: sanitizeLogLine(line),
		}})
	}

	if tty {
		// TTY containers produce a raw (non-multiplexed) stream.
		scanLines(rc, func(line string) { emit("stdout", line) })
		return
	}

	// Non-TTY: demux Docker's stdout/stderr framing.
	stdoutW := &lineWriter{emit: func(l string) { emit("stdout", l) }}
	stderrW := &lineWriter{emit: func(l string) { emit("stderr", l) }}
	_, _ = stdcopy.StdCopy(stdoutW, stderrW, rc)
	stdoutW.flush()
	stderrW.flush()
}

// sanitizeLogLine strips terminal escape sequences and stray control bytes from
// a container log line. Container output is attacker-controlled the moment any
// container on the host is compromised; without this, escape sequences (cursor
// moves, OSC title/clipboard injection, SGR color) would pass straight through
// to the browser log view. The UI renders text nodes and does its own severity
// coloring, so dropping color codes too is fine — we keep printable text and
// tabs, nothing else.
func sanitizeLogLine(s string) string {
	// Fast path: no ESC and no stray control bytes → return as-is.
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
		// Trim a trailing CR (Windows line endings in container output).
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		w.emit(string(line))
		w.buf = w.buf[i+1:]
	}
	// Guard against an unbounded line with no newline.
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
