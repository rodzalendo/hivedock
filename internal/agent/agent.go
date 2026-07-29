// Package agent is the remote side of HiveDock multi-host (docs/MULTIHOST.md):
// `hivedock agent` dials OUT to a manager over one WebSocket and answers its RPC
// by talking to the local Docker socket. Outbound-only — the remote host opens
// no inbound port. Phase 1 is read-only (list calls); write methods land later.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/rogalinski/hivedock/internal/agentrpc"
	"github.com/rogalinski/hivedock/internal/docker"
)

// Options configures the agent. ManagerURL is the manager's base URL (https/wss
// or http/ws); Token must match the manager's AGENT_TOKEN; Name is how this host
// appears in the manager.
type Options struct {
	ManagerURL string
	Token      string
	Name       string
	Version    string
	Logger     *slog.Logger
	Docker     *docker.Client
}

const (
	readTimeout = 90 * time.Second
	writeWait   = 10 * time.Second
)

// Run connects to the manager and serves RPC until ctx is cancelled, reconnecting
// with capped exponential backoff whenever the socket drops.
func Run(ctx context.Context, o Options) error {
	if o.ManagerURL == "" || o.Token == "" || o.Name == "" {
		return fmt.Errorf("agent: --manager, --token and --name are all required")
	}
	wsURL, err := connectURL(o.ManagerURL)
	if err != nil {
		return err
	}
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return nil
		}
		err := o.serve(ctx, wsURL)
		if ctx.Err() != nil {
			return nil
		}
		o.Logger.Warn("agent: disconnected, retrying", "err", err, "in", backoff.String())
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// serve runs one connection: dial, register, then a concurrent read / write /
// dispatch loop until the socket drops. A single writer goroutine drains `out`
// (gorilla allows one concurrent writer); each request is handled in its own
// goroutine so a long op (a deploy, a followed log) never blocks reads, and a
// `cancel` frame can abort an in-flight streaming call.
func (o Options) serve(ctx context.Context, wsURL string) error {
	hdr := http.Header{"Authorization": {"Bearer " + o.Token}}
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, hdr)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("dial %s: %w (http %d)", wsURL, err, resp.StatusCode)
		}
		return fmt.Errorf("dial %s: %w", wsURL, err)
	}
	defer conn.Close()
	o.Logger.Info("agent: connected to manager", "url", wsURL, "name", o.Name)

	// Opening hello, written before the writer goroutine starts (still single-
	// threaded at this point).
	reg, _ := json.Marshal(agentrpc.RegisterParams{Name: o.Name, Version: o.Version})
	if err := conn.WriteJSON(agentrpc.Request{Method: agentrpc.MethodRegister, Params: reg}); err != nil {
		return err
	}

	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ac := &agentConn{
		opts:     o,
		ws:       conn,
		out:      make(chan agentrpc.Response, 64),
		inflight: map[string]context.CancelFunc{},
	}
	writerDone := make(chan struct{})
	go ac.writeLoop(connCtx, writerDone)

	_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
	conn.SetPingHandler(func(msg string) error {
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		// WriteControl may run concurrently with the writer's WriteJSON (gorilla).
		return conn.WriteControl(websocket.PongMessage, []byte(msg), time.Now().Add(writeWait))
	})

	for {
		var req agentrpc.Request
		if err := conn.ReadJSON(&req); err != nil {
			cancel()
			<-writerDone
			return err
		}
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		ac.route(connCtx, req)
	}
}

// agentConn owns one manager connection: the sole-writer channel and the table of
// cancellable in-flight streaming calls.
type agentConn struct {
	opts Options
	ws   *websocket.Conn
	out  chan agentrpc.Response

	mu       sync.Mutex
	inflight map[string]context.CancelFunc
}

func (ac *agentConn) writeLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ac.out:
			_ = ac.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ac.ws.WriteJSON(msg); err != nil {
				return
			}
		}
	}
}

// send queues a frame for the writer, giving up if the connection is tearing down.
func (ac *agentConn) send(ctx context.Context, resp agentrpc.Response) {
	select {
	case ac.out <- resp:
	case <-ctx.Done():
	}
}

func (ac *agentConn) fail(ctx context.Context, id string, err error) {
	ac.send(ctx, agentrpc.Response{ID: id, Error: err.Error()})
}

// route dispatches a frame: control frames act on in-flight calls; every other
// method runs in its own goroutine so it can't block the read loop.
func (ac *agentConn) route(ctx context.Context, req agentrpc.Request) {
	switch req.Method {
	case agentrpc.MethodCancel:
		var p agentrpc.CancelParams
		if json.Unmarshal(req.Params, &p) == nil {
			ac.cancel(p.ID)
		}
		return
	case agentrpc.MethodExecInput, agentrpc.MethodExecResize:
		// Routed to the running exec stream in a later step; ignored until then.
		return
	}
	go ac.handle(ctx, req)
}

// handle runs one unary/streaming op against the agent's local host. Phase-2 stack
// methods are wired in a later step; today only ping/listContainers answer.
func (ac *agentConn) handle(ctx context.Context, req agentrpc.Request) {
	o := ac.opts
	switch req.Method {
	case agentrpc.MethodPing:
		ac.send(ctx, agentrpc.Response{ID: req.ID, Result: json.RawMessage(`"pong"`)})
	case agentrpc.MethodListContainers:
		cs, err := o.listContainers(ctx)
		if err != nil {
			ac.fail(ctx, req.ID, err)
			return
		}
		b, _ := json.Marshal(cs)
		ac.send(ctx, agentrpc.Response{ID: req.ID, Result: b})
	default:
		ac.fail(ctx, req.ID, fmt.Errorf("unknown method: %s", req.Method))
	}
}

// cancel aborts an in-flight streaming call by id (no-op if it already finished).
func (ac *agentConn) cancel(id string) {
	ac.mu.Lock()
	c := ac.inflight[id]
	ac.mu.Unlock()
	if c != nil {
		c()
	}
}

func (o Options) listContainers(ctx context.Context) ([]agentrpc.RemoteContainer, error) {
	if o.Docker == nil {
		return nil, fmt.Errorf("docker unavailable on this agent")
	}
	list, err := o.Docker.ListContainers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]agentrpc.RemoteContainer, 0, len(list))
	for _, c := range list {
		if c.Oneoff {
			continue
		}
		out = append(out, agentrpc.RemoteContainer{
			Name:    c.Name,
			Image:   c.Image,
			State:   c.State,
			Status:  c.Status,
			Health:  c.Health,
			Stack:   c.Project,
			Service: c.Service,
		})
	}
	return out, nil
}

// connectURL turns a manager base URL into the agent-connect WebSocket URL,
// defaulting to TLS when no scheme is given.
func connectURL(base string) (string, error) {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	switch {
	case strings.HasPrefix(b, "https://"):
		b = "wss://" + strings.TrimPrefix(b, "https://")
	case strings.HasPrefix(b, "http://"):
		b = "ws://" + strings.TrimPrefix(b, "http://")
	case strings.HasPrefix(b, "wss://"), strings.HasPrefix(b, "ws://"):
		// already a WebSocket URL
	default:
		b = "wss://" + b
	}
	u, err := url.Parse(b)
	if err != nil {
		return "", fmt.Errorf("invalid --manager URL %q: %w", base, err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/agent/connect"
	return u.String(), nil
}
