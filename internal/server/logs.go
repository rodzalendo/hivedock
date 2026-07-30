package server

import (
	"context"

	"github.com/rogalinski/hivedock/internal/events"
	"github.com/rogalinski/hivedock/internal/hostops"
)

// logLine is one streamed log line, tagged with its host, service and stream so
// the browser can attribute it to the right host's stack.
type logLine struct {
	Host    string `json:"host,omitempty"`
	Stack   string `json:"stack"`
	Service string `json:"service"`
	Stream  string `json:"stream"` // stdout | stderr
	Line    string `json:"line"`
}

// startLogs streams a stack's container logs to the client over the session
// socket, routed to the stack's host backend (the local docker for "local", else
// a remote agent). Lines are pushed as logs:line tagged by host. Re-subscribing to
// the same (host,stack) restarts it. Closing the session (or unsubscribing)
// cancels the stream — which, for a remote host, propagates a cancel to the agent
// so it stops its follow.
func (s *wsSession) startLogs(ctx context.Context, host, stackName string, tail int) {
	if stackName == "" {
		return
	}
	if host == "" {
		host = "local"
	}
	be, err := s.api.backendFor(host)
	if err != nil {
		s.send(events.Message{Type: "logs:error", Payload: map[string]string{
			"host": host, "stack": stackName, "message": err.Error(),
		}})
		return
	}

	// Cancel any existing stream for this (host,stack) before starting fresh.
	s.stopLogs(host, stackName)

	key := host + "/" + stackName
	streamCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		return
	}
	s.streams[key] = cancel
	s.mu.Unlock()

	go func() {
		err := be.Logs(streamCtx, stackName, tail, func(ll hostops.LogLine) {
			s.send(events.Message{Type: "logs:line", Payload: logLine{
				Host: host, Stack: stackName, Service: ll.Service, Stream: ll.Stream, Line: ll.Line,
			}})
		})
		// A real error (no running containers, host offline) is surfaced unless the
		// stream was cancelled (unsubscribe / disconnect), which is not an error.
		if err != nil && streamCtx.Err() == nil {
			s.send(events.Message{Type: "logs:error", Payload: map[string]string{
				"host": host, "stack": stackName, "message": err.Error(),
			}})
		}
		s.send(events.Message{Type: "logs:end", Payload: map[string]string{"host": host, "stack": stackName}})
	}()
}

func (s *wsSession) stopLogs(host, stackName string) {
	if host == "" {
		host = "local"
	}
	key := host + "/" + stackName
	s.mu.Lock()
	if cancel, ok := s.streams[key]; ok {
		cancel()
		delete(s.streams, key)
	}
	s.mu.Unlock()
}
