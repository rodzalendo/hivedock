package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/rogalinski/hivedock/internal/agentrpc"
	"github.com/rogalinski/hivedock/internal/docker"
)

// containerExec upgrades to a WebSocket and runs an interactive shell INSIDE the
// selected container (docker exec — never a host shell), on the local host or, with
// ?host=<name>, on a connected agent. It is auth-gated (it lives in the requireAuth
// group) and refused when a boot check put the server in read-only mode, matching
// how every other privileged action is gated. A remote host runs the same fixed
// shell and enforces its own read-only mode on the agent. Each session is logged.
func (a *api) containerExec(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	if host == "" {
		host = "local"
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing container id")
		return
	}
	shell := r.URL.Query().Get("shell")

	if host == "local" {
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
	} else if a.hosts.get(host) == nil {
		writeError(w, http.StatusBadGateway, "host is offline: "+host)
		return
	}

	up := upgrader
	up.CheckOrigin = a.checkWSOrigin
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		a.logger.Warn("exec ws upgrade failed", "err", err, "remote", r.RemoteAddr)
		return
	}
	defer conn.Close()

	a.logger.Info("container exec session", "host", host, "container", id, "shell", shell, "remote", r.RemoteAddr)
	if host == "local" {
		a.runContainerExec(r.Context(), conn, id, docker.ShellCommand(shell))
	} else {
		a.runRemoteExec(r.Context(), conn, host, id, shell)
	}
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

	a.pumpExecSocket(ctx, cancel, conn, out,
		func(data []byte) bool { _, err := hj.Conn.Write(data); return err == nil },
		func(rows, cols uint) { _ = a.docker.ExecResize(ctx, execID, rows, cols) })
}

// runRemoteExec tunnels an exec to an agent over the RPC: the agent opens the
// shell against its own docker and streams output as chunk frames; the manager
// relays browser stdin as execInput frames and resizes as execResize frames, and a
// closed browser socket cancels the stream (which stops the agent's exec).
func (a *api) runRemoteExec(ctx context.Context, conn *websocket.Conn, host, id, shell string) {
	h := a.hosts.get(host)
	if h == nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31mhost is offline\x1b[0m\r\n"))
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch, streamID, err := h.CallStream(ctx, agentrpc.MethodExec, agentrpc.ExecParams{ContainerID: id, Shell: shell})
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31mcould not open a shell: "+err.Error()+"\x1b[0m\r\n"))
		return
	}

	// Agent frames → out channel: chunk = terminal bytes; terminal frame ends it.
	out := make(chan []byte, 64)
	go func() {
		defer cancel()
		for resp := range ch {
			if resp.Kind != agentrpc.KindStream {
				if resp.Error != "" {
					select {
					case out <- []byte("\r\n\x1b[31m" + resp.Error + "\x1b[0m\r\n"):
					case <-ctx.Done():
					}
				}
				return
			}
			var b []byte
			if json.Unmarshal(resp.Result, &b) != nil {
				continue
			}
			select {
			case out <- b:
			case <-ctx.Done():
				return
			}
		}
	}()

	a.pumpExecSocket(ctx, cancel, conn, out,
		func(data []byte) bool {
			return h.sendFrame(agentrpc.MethodExecInput, agentrpc.ExecInputParams{ID: streamID, Data: data}) == nil
		},
		func(rows, cols uint) {
			_ = h.sendFrame(agentrpc.MethodExecResize, agentrpc.ExecResizeParams{ID: streamID, Rows: rows, Cols: cols})
		})
}

// pumpExecSocket is the shared exec plumbing: a single writer goroutine drains out
// to the browser (binary frames) with keepalive pings, while the read loop routes
// browser binary frames to writeStdin and resize controls to resize. Any error
// cancels the session. Used by both the local and remote exec paths.
func (a *api) pumpExecSocket(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, out <-chan []byte, writeStdin func([]byte) bool, resize func(rows, cols uint)) {
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

	conn.SetReadLimit(1 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return // client closed; the deferred cancel stops the exec
		}
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		switch mt {
		case websocket.BinaryMessage:
			if !writeStdin(data) {
				return
			}
		case websocket.TextMessage:
			var ctl execControl
			if json.Unmarshal(data, &ctl) == nil && ctl.Type == "resize" && ctl.Cols > 0 && ctl.Rows > 0 {
				resize(ctl.Rows, ctl.Cols)
			}
		}
	}
}
