package server

import (
	"bufio"
	"bytes"
	"context"
	"io"
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
			Stack: stack, Service: service, Stream: stream, Line: line,
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
