package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/rogalinski/hivedock/internal/events"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Same-origin is enforced by default (Origin must match Host). vite proxies
	// /api to the Go server in dev, so no override is needed.
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	outBuffer  = 256
)

// clientCommand is a message sent by the browser over the socket.
type clientCommand struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// wsSession owns one client connection. A single writer goroutine drains `out`
// (gorilla requires single-threaded writes); hub events and log lines are
// funneled into `out` from other goroutines.
type wsSession struct {
	api  *api
	conn *websocket.Conn
	out  chan events.Message

	mu      sync.Mutex
	streams map[string]context.CancelFunc // stack name -> cancel its log streams
	closed  bool
}

// websocket upgrades the connection and runs the session until disconnect.
func (a *api) websocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		a.logger.Warn("ws upgrade failed", "err", err, "remote", r.RemoteAddr)
		return
	}
	defer conn.Close()

	s := &wsSession{
		api:     a,
		conn:    conn,
		out:     make(chan events.Message, outBuffer),
		streams: map[string]context.CancelFunc{},
	}
	a.logger.Debug("ws connected", "remote", r.RemoteAddr)
	s.run(r.Context())
	a.logger.Debug("ws disconnected", "remote", r.RemoteAddr)
}

func (s *wsSession) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer s.cancelAllStreams()

	// Forward hub notifications (state changes) into the outbound channel.
	sub, unsub := s.api.hub.Subscribe()
	defer unsub()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-sub:
				if !ok {
					return
				}
				s.send(msg)
			}
		}
	}()

	s.send(events.Message{Type: "hello", Payload: map[string]string{
		"version": version,
		"time":    time.Now().UTC().Format(time.RFC3339),
	}})

	go s.writeLoop(ctx, cancel)
	s.readLoop(ctx) // blocks until the client disconnects
}

// send queues a message for the writer. Non-blocking: if the client is too slow
// and the buffer is full, the connection is torn down (a stalled log viewer
// shouldn't wedge the server).
func (s *wsSession) send(msg events.Message) {
	select {
	case s.out <- msg:
	default:
		s.close()
	}
}

func (s *wsSession) writeLoop(ctx context.Context, cancel context.CancelFunc) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	defer cancel() // a write failure ends the whole session

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-s.out:
			if !ok {
				return
			}
			_ = s.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := s.conn.WriteJSON(msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = s.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := s.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *wsSession) readLoop(ctx context.Context) {
	s.conn.SetReadLimit(4096)
	_ = s.conn.SetReadDeadline(time.Now().Add(pongWait))
	s.conn.SetPongHandler(func(string) error {
		return s.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := s.conn.ReadMessage()
		if err != nil {
			return
		}
		var cmd clientCommand
		if err := json.Unmarshal(data, &cmd); err != nil {
			continue
		}
		s.dispatch(ctx, cmd)
	}
}

func (s *wsSession) dispatch(ctx context.Context, cmd clientCommand) {
	switch cmd.Type {
	case "logs:subscribe":
		var p struct {
			Stack string `json:"stack"`
			Tail  int    `json:"tail"`
		}
		_ = json.Unmarshal(cmd.Payload, &p)
		s.startLogs(ctx, p.Stack, p.Tail)
	case "logs:unsubscribe":
		var p struct {
			Stack string `json:"stack"`
		}
		_ = json.Unmarshal(cmd.Payload, &p)
		s.stopLogs(p.Stack)
	}
}

func (s *wsSession) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	_ = s.conn.Close() // unblocks readLoop
}

func (s *wsSession) cancelAllStreams() {
	s.mu.Lock()
	for _, cancel := range s.streams {
		cancel()
	}
	s.streams = map[string]context.CancelFunc{}
	s.mu.Unlock()
}
