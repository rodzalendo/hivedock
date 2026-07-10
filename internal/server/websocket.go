package server

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// upgrader accepts same-origin connections. Phase 0 only proves the socket
// works end-to-end; the multiplexed protocol (events, logs, deploy output,
// stats) lands in Phase 1.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Same-origin is enforced by default when Origin matches Host; a dev-server
	// override is unnecessary because vite proxies /api to the Go server.
}

type wsMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

func (a *api) websocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		a.logger.Warn("ws upgrade failed", "err", err, "remote", r.RemoteAddr)
		return
	}
	defer conn.Close()

	a.logger.Debug("ws connected", "remote", r.RemoteAddr)

	if err := conn.WriteJSON(wsMessage{Type: "hello", Payload: map[string]string{
		"version": version,
		"time":    time.Now().UTC().Format(time.RFC3339),
	}}); err != nil {
		return
	}

	// Drain reads so control frames (ping/pong/close) are handled until the
	// client disconnects. Real subscriptions arrive in Phase 1.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			a.logger.Debug("ws closed", "remote", r.RemoteAddr, "err", err)
			return
		}
	}
}
