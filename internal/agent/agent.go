// Package agent is the remote side of HiveDock multi-host (docs/MULTIHOST.md):
// `hivedock agent` dials OUT to a manager over one WebSocket and answers its RPC
// by talking to the local Docker socket. Outbound-only — the remote host opens
// no inbound port. Phase 1 is read-only (list calls); write methods land later.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/rogalinski/hivedock/internal/agentrpc"
	"github.com/rogalinski/hivedock/internal/compose"
	"github.com/rogalinski/hivedock/internal/docker"
	"github.com/rogalinski/hivedock/internal/hostops"
	"github.com/rogalinski/hivedock/internal/stacks"
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

	// StacksDir is the compose stacks root this agent manages (its STACKS_DIR).
	// The agent runs the same hostops LocalBackend the manager does, so remote
	// stack management shares the manager's security invariants (docs/MULTIHOST.md).
	StacksDir string
	// ReadOnlyReason, when non-empty, makes the agent refuse every mutating method
	// (a bind-mismatched STACKS_DIR, §6.4) — set by the boot syscheck.
	ReadOnlyReason string
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
	// The agent's own stack-management backend: the identical hostops core the
	// manager runs, over this host's STACKS_DIR + docker socket. git is nil (git
	// auto-commit stays a manager-only feature). The lister is nil-guarded to avoid
	// a typed-nil interface wrapping a nil *docker.Client.
	var lister stacks.ContainerLister
	if o.Docker != nil {
		lister = o.Docker
	}
	local := hostops.NewLocal(o.StacksDir, stacks.NewManager(o.StacksDir, lister, o.Logger), compose.NewRunner(), o.Docker, nil, o.Logger)
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return nil
		}
		err := o.serve(ctx, wsURL, local)
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
func (o Options) serve(ctx context.Context, wsURL string, local *hostops.LocalBackend) error {
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
		local:    local,
		out:      make(chan agentrpc.Response, 64),
		inflight: map[string]context.CancelFunc{},
		execs:    map[string]*execSession{},
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
	opts  Options
	ws    *websocket.Conn
	local *hostops.LocalBackend
	out   chan agentrpc.Response

	mu       sync.Mutex
	inflight map[string]context.CancelFunc // streaming call id -> cancel
	execs    map[string]*execSession       // exec stream id -> live shell (for stdin/resize)
}

// execSession is one running remote exec: its hijacked stdin writer and the exec
// ID used to resize the TTY.
type execSession struct {
	stdin  io.Writer
	execID string
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
	case agentrpc.MethodExecInput:
		var p agentrpc.ExecInputParams
		if json.Unmarshal(req.Params, &p) == nil {
			ac.execInput(p.ID, p.Data)
		}
		return
	case agentrpc.MethodExecResize:
		var p agentrpc.ExecResizeParams
		if json.Unmarshal(req.Params, &p) == nil {
			ac.execResize(p.ID, p.Rows, p.Cols)
		}
		return
	}
	go ac.handle(ctx, req)
}

// handle runs one op against the agent's local host, routing each method to the
// shared hostops backend. Mutating methods are gated by the agent's own read-only
// reason and (for the compose-shelling ops) serialized by the local per-stack lock
// — the same invariants the manager enforces, run here on the file-owning host
// (docs/MULTIHOST.md).
func (ac *agentConn) handle(ctx context.Context, req agentrpc.Request) {
	switch req.Method {
	case agentrpc.MethodPing:
		ac.send(ctx, agentrpc.Response{ID: req.ID, Result: json.RawMessage(`"pong"`)})
	case agentrpc.MethodListContainers:
		cs, err := ac.opts.listContainers(ctx)
		ac.reply(ctx, req.ID, cs, err)

	// --- reads ---
	case agentrpc.MethodListStacks:
		list, err := ac.local.ListStacks(ctx)
		ac.reply(ctx, req.ID, list, err)
	case agentrpc.MethodGetStack:
		var p agentrpc.NameParam
		_ = json.Unmarshal(req.Params, &p)
		st, err := ac.local.GetStack(ctx, p.Name)
		ac.reply(ctx, req.ID, st, err)
	case agentrpc.MethodGetCompose:
		var p agentrpc.NameParam
		_ = json.Unmarshal(req.Params, &p)
		cf, err := ac.local.GetCompose(ctx, p.Name)
		ac.reply(ctx, req.ID, cf, err)
	case agentrpc.MethodValidateCompose:
		var p agentrpc.ComposeWriteParams
		_ = json.Unmarshal(req.Params, &p)
		v, err := ac.local.ValidateCompose(ctx, p.Name, []byte(p.Content))
		ac.reply(ctx, req.ID, v, err)
	case agentrpc.MethodGetEnv:
		var p agentrpc.NameParam
		_ = json.Unmarshal(req.Params, &p)
		ef, err := ac.local.GetEnv(ctx, p.Name)
		ac.reply(ctx, req.ID, ef, err)

	// --- file writes (gated) ---
	case agentrpc.MethodPutCompose:
		if ac.gated(ctx, req.ID) {
			return
		}
		var p agentrpc.ComposeWriteParams
		_ = json.Unmarshal(req.Params, &p)
		cf, err := ac.local.PutCompose(ctx, p.Name, []byte(p.Content), p.BaseSha256)
		ac.reply(ctx, req.ID, cf, err)
	case agentrpc.MethodPutEnv:
		if ac.gated(ctx, req.ID) {
			return
		}
		var p agentrpc.ComposeWriteParams
		_ = json.Unmarshal(req.Params, &p)
		ef, err := ac.local.PutEnv(ctx, p.Name, []byte(p.Content), p.BaseSha256)
		ac.reply(ctx, req.ID, ef, err)
	case agentrpc.MethodCreateStack:
		if ac.gated(ctx, req.ID) {
			return
		}
		var p agentrpc.CreateStackParams
		_ = json.Unmarshal(req.Params, &p)
		ref, err := ac.local.CreateStack(ctx, p.Name, p.Compose)
		ac.reply(ctx, req.ID, ref, err)
	case agentrpc.MethodRenameStack:
		if ac.gated(ctx, req.ID) {
			return
		}
		var p agentrpc.RenameStackParams
		_ = json.Unmarshal(req.Params, &p)
		ref, err := ac.local.RenameStack(ctx, p.Name, p.NewName)
		ac.reply(ctx, req.ID, ref, err)
	case agentrpc.MethodUpdateService:
		if ac.gated(ctx, req.ID) {
			return
		}
		var p agentrpc.UpdateServiceParams
		_ = json.Unmarshal(req.Params, &p)
		res, err := ac.local.UpdateService(ctx, hostops.UpdateServiceReq{
			Name: p.Name, Service: p.Service, Tag: p.Tag, BaseSha256: p.BaseSha256, Confirm: p.Confirm,
		})
		ac.reply(ctx, req.ID, res, err)
	case agentrpc.MethodDeleteStack:
		if ac.gated(ctx, req.ID) {
			return
		}
		var p agentrpc.DeleteStackParams
		_ = json.Unmarshal(req.Params, &p)
		release, ok := ac.local.Runner().Start(p.Name)
		if !ok {
			ac.sendTerminal(ctx, req.ID, hostops.ErrBusy)
			return
		}
		defer release()
		ac.reply(ctx, req.ID, nil, ac.local.DeleteStack(ctx, p.Name, p.Volumes))

	// --- streaming ---
	case agentrpc.MethodRunAction:
		ac.streamRunAction(ctx, req)
	case agentrpc.MethodLogs:
		ac.streamLogs(ctx, req)
	case agentrpc.MethodExec:
		ac.streamExec(ctx, req)

	default:
		ac.fail(ctx, req.ID, fmt.Errorf("unknown method: %s", req.Method))
	}
}

// gated refuses a mutating method when the agent is read-only, sending the typed
// error terminal and returning true (the caller should stop).
func (ac *agentConn) gated(ctx context.Context, id string) bool {
	if ac.opts.ReadOnlyReason != "" {
		ac.sendTerminal(ctx, id, hostops.ErrReadOnly)
		return true
	}
	return false
}

// reply sends a unary terminal: the marshaled value on success, or the typed
// error (mapped to a wire code) on failure. A nil value carries no Result.
func (ac *agentConn) reply(ctx context.Context, id string, value any, err error) {
	if err != nil {
		ac.sendTerminal(ctx, id, err)
		return
	}
	resp := agentrpc.Response{ID: id}
	if value != nil {
		resp.Result, _ = json.Marshal(value)
	}
	ac.send(ctx, resp)
}

// sendTerminal sends the final frame of a call: a bare terminal on success, or an
// error terminal carrying the machine Code (and, for a conflict, the current
// bytes) so the manager reproduces the same HTTP status a local failure would.
func (ac *agentConn) sendTerminal(ctx context.Context, id string, err error) {
	if err == nil {
		ac.send(ctx, agentrpc.Response{ID: id})
		return
	}
	resp := agentrpc.Response{ID: id, Code: hostops.CodeFor(err), Error: err.Error()}
	if conflict, ok := err.(*hostops.ConflictError); ok {
		resp.Result, _ = json.Marshal(conflict)
	}
	ac.send(ctx, resp)
}

// streamRunAction runs a compose action and streams its output lines as chunk
// frames, then a terminal. Gated by read-only and serialized by the per-stack
// lock; a cancel frame for this ID aborts it.
func (ac *agentConn) streamRunAction(ctx context.Context, req agentrpc.Request) {
	if ac.gated(ctx, req.ID) {
		return
	}
	var p agentrpc.RunActionParams
	_ = json.Unmarshal(req.Params, &p)
	release, ok := ac.local.Runner().Start(p.Name)
	if !ok {
		ac.sendTerminal(ctx, req.ID, hostops.ErrBusy)
		return
	}
	defer release()

	streamCtx := ac.trackCancel(ctx, req.ID)
	defer ac.untrack(req.ID)

	err := ac.local.RunAction(streamCtx, p.Name, p.Action, p.Service, func(line string) {
		b, _ := json.Marshal(line)
		ac.send(ctx, agentrpc.Response{ID: req.ID, Kind: agentrpc.KindStream, Result: b})
	})
	ac.sendTerminal(ctx, req.ID, err)
}

// streamLogs follows a stack's container logs, streaming each line as a chunk
// frame until the containers stop or the manager cancels (unsubscribe / client
// disconnect). A read — no gate, no lock.
func (ac *agentConn) streamLogs(ctx context.Context, req agentrpc.Request) {
	var p agentrpc.LogsParams
	_ = json.Unmarshal(req.Params, &p)

	streamCtx := ac.trackCancel(ctx, req.ID)
	defer ac.untrack(req.ID)

	err := ac.local.Logs(streamCtx, p.Stack, p.Tail, func(ll hostops.LogLine) {
		b, _ := json.Marshal(ll)
		ac.send(ctx, agentrpc.Response{ID: req.ID, Kind: agentrpc.KindStream, Result: b})
	})
	ac.sendTerminal(ctx, req.ID, err)
}

// trackCancel registers a cancellable child context for a streaming call so a
// `cancel` frame for its ID can abort it.
func (ac *agentConn) trackCancel(ctx context.Context, id string) context.Context {
	streamCtx, cancel := context.WithCancel(ctx)
	ac.mu.Lock()
	ac.inflight[id] = cancel
	ac.mu.Unlock()
	return streamCtx
}

func (ac *agentConn) untrack(id string) {
	ac.mu.Lock()
	cancel := ac.inflight[id]
	delete(ac.inflight, id)
	ac.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// streamExec opens an interactive shell inside a container on this host and
// streams its TTY output as chunk frames (raw bytes, base64 on the wire). Browser
// stdin and resizes arrive as execInput/execResize control frames routed by ID; a
// cancel (or the browser socket closing) ends it. Exec can mutate a container, so
// it is gated by read-only exactly like a write.
func (ac *agentConn) streamExec(ctx context.Context, req agentrpc.Request) {
	if ac.gated(ctx, req.ID) {
		return
	}
	var p agentrpc.ExecParams
	_ = json.Unmarshal(req.Params, &p)
	if ac.opts.Docker == nil {
		ac.sendTerminal(ctx, req.ID, hostops.ErrNoDocker)
		return
	}

	streamCtx := ac.trackCancel(ctx, req.ID)
	defer ac.untrack(req.ID)

	hj, execID, err := ac.opts.Docker.ExecAttach(streamCtx, p.ContainerID, docker.ShellCommand(p.Shell))
	if err != nil {
		ac.sendTerminal(ctx, req.ID, err)
		return
	}
	defer hj.Close()

	ac.mu.Lock()
	ac.execs[req.ID] = &execSession{stdin: hj.Conn, execID: execID}
	ac.mu.Unlock()
	defer func() {
		ac.mu.Lock()
		delete(ac.execs, req.ID)
		ac.mu.Unlock()
	}()

	// Cancelling the stream must unblock the blocking read below.
	go func() {
		<-streamCtx.Done()
		hj.Close()
	}()

	buf := make([]byte, 4096)
	for {
		n, rerr := hj.Reader.Read(buf)
		if n > 0 {
			b := make([]byte, n)
			copy(b, buf[:n])
			data, _ := json.Marshal(b)
			ac.send(ctx, agentrpc.Response{ID: req.ID, Kind: agentrpc.KindStream, Result: data})
		}
		if rerr != nil {
			break
		}
	}
	ac.sendTerminal(ctx, req.ID, nil)
}

// execInput writes browser stdin bytes to a running exec's TTY.
func (ac *agentConn) execInput(id string, data []byte) {
	ac.mu.Lock()
	es := ac.execs[id]
	ac.mu.Unlock()
	if es != nil {
		_, _ = es.stdin.Write(data)
	}
}

// execResize resizes a running exec's TTY.
func (ac *agentConn) execResize(id string, rows, cols uint) {
	ac.mu.Lock()
	es := ac.execs[id]
	ac.mu.Unlock()
	if es != nil && ac.opts.Docker != nil {
		_ = ac.opts.Docker.ExecResize(context.Background(), es.execID, rows, cols)
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
