package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// shellCmd maps the requested shell to a FIXED command. The default prefers bash
// and falls back to sh, resolved inside the container. Crucially the returned
// slice is never built from arbitrary user input — only these three known
// commands are possible — so there is no command-injection surface (invariant 9:
// no shell metacharacters we didn't write ourselves).
func shellCmd(shell string) []string {
	switch shell {
	case "bash":
		return []string{"/bin/bash"}
	case "sh":
		return []string{"/bin/sh"}
	default:
		// Prefer bash, fall back to sh — but check for bash BEFORE exec: a failed
		// `exec` makes a POSIX shell exit immediately (it would never reach a
		// `|| exec sh` fallback), so guard it with `command -v` first.
		return []string{"/bin/sh", "-c", "if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi"}
	}
}

// containerExec upgrades to a WebSocket and runs an interactive shell INSIDE the
// selected container (docker exec — never a host shell). It is auth-gated (it
// lives in the requireAuth group) and refused when a boot check put the server
// in read-only mode, matching how every other privileged action is gated. Each
// session is logged.
func (a *api) containerExec(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		writeError(w, http.StatusServiceUnavailable, "docker is unavailable")
		return
	}
	// Exec can mutate a container, so it is refused in read-only mode just like a
	// write. (enforceReadOnly only blocks unsafe HTTP methods; a WS upgrade is a
	// GET, so the check has to be explicit here.)
	if a.readOnlyReason != "" {
		writeError(w, http.StatusServiceUnavailable, a.readOnlyReason)
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing container id")
		return
	}
	cmd := shellCmd(r.URL.Query().Get("shell"))

	up := upgrader
	up.CheckOrigin = a.checkWSOrigin
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		a.logger.Warn("exec ws upgrade failed", "err", err, "remote", r.RemoteAddr)
		return
	}
	defer conn.Close()

	a.logger.Info("container exec session", "container", id, "shell", strings.Join(cmd, " "), "remote", r.RemoteAddr)
	a.runContainerExec(r.Context(), conn, id, cmd)
}

// execControl is a client → server control frame (sent as a WS text message).
// Terminal input travels as binary frames; only resize is modeled here.
type execControl struct {
	Type string `json:"type"`
	Cols uint   `json:"cols"`
	Rows uint   `json:"rows"`
}

// runContainerExec wires the hijacked docker exec stream to the WebSocket:
// binary frames carry terminal bytes both ways; text frames carry resize
// controls. One goroutine owns all socket writes (gorilla requires a single
// writer); any error tears the whole session down.
func (a *api) runContainerExec(ctx context.Context, conn *websocket.Conn, id string, cmd []string) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	hj, execID, err := a.docker.ExecAttach(ctx, id, cmd)
	if err != nil {
		// Surface the reason in the terminal itself (red), then close.
		_ = conn.WriteMessage(websocket.TextMessage,
			[]byte("\r\n\x1b[31mcould not open a shell: "+err.Error()+"\x1b[0m\r\n"))
		return
	}
	defer hj.Close()

	// container stdout/stderr (merged under a TTY) → out channel.
	out := make(chan []byte, 64)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := hj.Reader.Read(buf)
			if n > 0 {
				b := make([]byte, n)
				copy(b, buf[:n])
				select {
				case out <- b:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				cancel() // shell exited or stream broke
				return
			}
		}
	}()

	// Sole socket writer: terminal bytes + keepalive pings.
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case b, ok := <-out:
				if !ok {
					return
				}
				_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.BinaryMessage, b); err != nil {
					cancel()
					return
				}
			case <-ticker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// Client → container: binary = stdin, text = control (resize).
	conn.SetReadLimit(1 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return // client closed; defers cancel + close the exec
		}
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		switch mt {
		case websocket.BinaryMessage:
			if _, err := hj.Conn.Write(data); err != nil {
				return
			}
		case websocket.TextMessage:
			var ctl execControl
			if json.Unmarshal(data, &ctl) == nil && ctl.Type == "resize" && ctl.Cols > 0 && ctl.Rows > 0 {
				_ = a.docker.ExecResize(ctx, execID, ctl.Rows, ctl.Cols)
			}
		}
	}
}
