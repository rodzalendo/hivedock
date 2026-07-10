package server

import (
	"net/http"
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
)

// websocket upgrades the connection and streams hub events (state-change
// notifications) to the client until it disconnects. The client refetches via
// REST when it receives a "stacks:changed" message.
func (a *api) websocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		a.logger.Warn("ws upgrade failed", "err", err, "remote", r.RemoteAddr)
		return
	}
	defer conn.Close()

	sub, unsub := a.hub.Subscribe()
	defer unsub()

	a.logger.Debug("ws connected", "remote", r.RemoteAddr)

	// Reader goroutine: handle pongs and detect disconnect.
	go a.wsReader(conn)

	_ = conn.WriteJSON(events.Message{Type: "hello", Payload: map[string]string{
		"version": version,
		"time":    time.Now().UTC().Format(time.RFC3339),
	}})

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-sub:
			if !ok {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteJSON(msg); err != nil {
				a.logger.Debug("ws write failed; closing", "remote", r.RemoteAddr, "err", err)
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (a *api) wsReader(conn *websocket.Conn) {
	conn.SetReadLimit(512)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			conn.Close() // unblocks the writer's next write
			return
		}
	}
}
