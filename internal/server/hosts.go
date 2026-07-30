package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/rogalinski/hivedock/internal/agentrpc"
)

// hostConn is one connected remote agent (docs/MULTIHOST.md). Manager → agent
// calls are correlated by id: unary calls wait on `pending`, streaming calls
// (deploy output, logs, exec) get a `streaming` entry that carries many chunk
// frames then a terminal. Writes are serialized (gorilla requires a single
// writer); reads happen only in readLoop.
type hostConn struct {
	name    string
	version string
	conn    *websocket.Conn

	wmu sync.Mutex // serialize socket writes

	mu        sync.Mutex
	seq       int
	pending   map[string]chan agentrpc.Response
	streaming map[string]*streamEntry
}

// streamEntry is an in-flight streaming call: chunk frames flow on ch; done is
// closed when the terminal frame arrives (or the socket drops) so the
// cancellation watcher can stop.
type streamEntry struct {
	ch   chan agentrpc.Response
	done chan struct{}
}

func newHostConn(name, version string, conn *websocket.Conn) *hostConn {
	return &hostConn{
		name: name, version: version, conn: conn,
		pending:   map[string]chan agentrpc.Response{},
		streaming: map[string]*streamEntry{},
	}
}

// writeReq writes one frame under the single-writer lock.
func (h *hostConn) writeReq(req agentrpc.Request) error {
	h.wmu.Lock()
	defer h.wmu.Unlock()
	_ = h.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return h.conn.WriteJSON(req)
}

// sendFrame sends a fire-and-forget control frame (cancel, exec stdin/resize).
func (h *hostConn) sendFrame(method string, params any) error {
	raw, err := marshalParams(params)
	if err != nil {
		return err
	}
	return h.writeReq(agentrpc.Request{Method: method, Params: raw})
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	return json.Marshal(params)
}

// Call sends a UNARY RPC and returns the agent's terminal reply. The returned
// error is transport-level only (couldn't send, socket dropped, ctx done); an
// application failure is carried in the Response (Error/Code).
func (h *hostConn) Call(ctx context.Context, method string, params any) (agentrpc.Response, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return agentrpc.Response{}, err
	}
	h.mu.Lock()
	h.seq++
	id := strconv.Itoa(h.seq)
	ch := make(chan agentrpc.Response, 1)
	h.pending[id] = ch
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, id)
		h.mu.Unlock()
	}()

	if err := h.writeReq(agentrpc.Request{ID: id, Method: method, Params: raw}); err != nil {
		return agentrpc.Response{}, err
	}
	select {
	case <-ctx.Done():
		return agentrpc.Response{}, ctx.Err()
	case resp := <-ch:
		return resp, nil
	}
}

// CallStream starts a STREAMING RPC. Frames arrive on the returned channel: each
// Kind==KindStream is a chunk; the final frame is terminal and the channel is
// then closed. The returned id lets the caller push follow-up control frames
// (exec stdin/resize) via sendFrame. Cancelling ctx sends a `cancel` to the agent.
func (h *hostConn) CallStream(ctx context.Context, method string, params any) (<-chan agentrpc.Response, string, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return nil, "", err
	}
	h.mu.Lock()
	h.seq++
	id := strconv.Itoa(h.seq)
	e := &streamEntry{ch: make(chan agentrpc.Response, 256), done: make(chan struct{})}
	h.streaming[id] = e
	h.mu.Unlock()

	if err := h.writeReq(agentrpc.Request{ID: id, Method: method, Params: raw}); err != nil {
		h.mu.Lock()
		delete(h.streaming, id)
		h.mu.Unlock()
		return nil, "", err
	}
	// Propagate cancellation to the agent; stop when the stream ends.
	go func() {
		select {
		case <-ctx.Done():
			_ = h.sendFrame(agentrpc.MethodCancel, agentrpc.CancelParams{ID: id})
		case <-e.done:
		}
	}()
	return e.ch, id, nil
}

// readLoop routes agent frames to their callers until the socket closes, then
// drains every in-flight call with a synthetic terminal so nothing hangs.
func (h *hostConn) readLoop() {
	defer h.drain()
	for {
		var resp agentrpc.Response
		if err := h.conn.ReadJSON(&resp); err != nil {
			return
		}
		if resp.Kind == agentrpc.KindStream {
			h.mu.Lock()
			e := h.streaming[resp.ID]
			h.mu.Unlock()
			if e != nil {
				select {
				case e.ch <- resp:
				default: // consumer slow; drop this chunk (bounded backpressure)
				}
			}
			continue
		}
		// Terminal frame: a streaming call's end, or a unary reply.
		h.mu.Lock()
		e := h.streaming[resp.ID]
		uch := h.pending[resp.ID]
		h.mu.Unlock()
		if e != nil {
			h.finishStream(resp.ID, resp)
		} else if uch != nil {
			uch <- resp
		}
	}
}

// finishStream delivers the terminal frame, closes the channel, and signals done.
func (h *hostConn) finishStream(id string, terminal agentrpc.Response) {
	h.mu.Lock()
	e := h.streaming[id]
	if e == nil {
		h.mu.Unlock()
		return
	}
	delete(h.streaming, id)
	h.mu.Unlock()
	select {
	case e.ch <- terminal:
	default:
	}
	close(e.ch)
	close(e.done)
}

// drain unblocks every in-flight call when the socket drops.
func (h *hostConn) drain() {
	h.mu.Lock()
	pending, streaming := h.pending, h.streaming
	h.pending = map[string]chan agentrpc.Response{}
	h.streaming = map[string]*streamEntry{}
	h.mu.Unlock()
	for id, ch := range pending {
		select {
		case ch <- agentrpc.Response{ID: id, Error: "host disconnected", Code: "offline"}:
		default:
		}
	}
	for _, e := range streaming {
		select {
		case e.ch <- agentrpc.Response{Error: "host disconnected", Code: "offline"}:
		default:
		}
		close(e.ch)
		close(e.done)
	}
}

// hostRegistry tracks live agent connections keyed by host name.
type hostRegistry struct {
	mu sync.RWMutex
	m  map[string]*hostConn
}

func newHostRegistry() *hostRegistry { return &hostRegistry{m: map[string]*hostConn{}} }

func (r *hostRegistry) add(h *hostConn) {
	r.mu.Lock()
	r.m[h.name] = h
	r.mu.Unlock()
}

// remove drops name only if it still points at h — so a reconnect that replaced
// the entry isn't clobbered by the old connection's cleanup.
func (r *hostRegistry) remove(name string, h *hostConn) {
	r.mu.Lock()
	if r.m[name] == h {
		delete(r.m, name)
	}
	r.mu.Unlock()
}

func (r *hostRegistry) get(name string) *hostConn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.m[name]
}

func (r *hostRegistry) list() []*hostConn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*hostConn, 0, len(r.m))
	for _, h := range r.m {
		out = append(out, h)
	}
	return out
}

// agentConnect is the enrollment endpoint remote agents dial. It is token-gated
// (NOT session-gated — agents carry no cookie) and disabled entirely unless
// AGENT_TOKEN is set. Cross-origin is allowed on purpose: the caller is another
// host proving itself with the bearer token, not a browser with ambient cookies.
func (a *api) agentConnect(w http.ResponseWriter, r *http.Request) {
	if !a.agentTokenConfigured() {
		writeError(w, http.StatusNotFound, "multi-host is not enabled (set AGENT_TOKEN or mint an agent token in Settings)")
		return
	}
	if !a.agentTokenAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "invalid agent token")
		return
	}
	up := upgrader
	up.CheckOrigin = func(*http.Request) bool { return true } // token-gated, cross-host by design
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		a.logger.Warn("agent connect: upgrade failed", "err", err, "remote", r.RemoteAddr)
		return
	}
	defer conn.Close()

	// The first frame must be the register hello.
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var hello agentrpc.Request
	if err := conn.ReadJSON(&hello); err != nil || hello.Method != agentrpc.MethodRegister {
		a.logger.Warn("agent connect: expected register hello", "err", err, "method", hello.Method)
		return
	}
	var reg agentrpc.RegisterParams
	_ = json.Unmarshal(hello.Params, &reg)
	name := sanitizeHostName(reg.Name)
	if name == "" {
		a.logger.Warn("agent connect: empty/invalid host name", "raw", reg.Name)
		return
	}

	h := newHostConn(name, reg.Version, conn)
	a.hosts.add(h)
	a.logger.Info("agent connected", "host", name, "version", reg.Version, "remote", r.RemoteAddr)
	a.hub.NotifyChanged("hosts:changed")
	defer func() {
		a.hosts.remove(name, h)
		a.logger.Info("agent disconnected", "host", name)
		a.hub.NotifyChanged("hosts:changed")
	}()

	// Keepalive: ping the agent; refresh the read deadline on its pong.
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(pongWait)) })
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(pingPeriod)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				h.wmu.Lock()
				_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
				err := conn.WriteMessage(websocket.PingMessage, nil)
				h.wmu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()
	h.readLoop()
	close(stop)
}

// hostInfo is the manager's view of a host for the UI.
type hostInfo struct {
	Name    string `json:"name"`
	Local   bool   `json:"local"`
	Online  bool   `json:"online"`
	Version string `json:"version,omitempty"`
}

// listHosts returns the local host plus every connected agent.
func (a *api) listHosts(w http.ResponseWriter, r *http.Request) {
	hosts := []hostInfo{{Name: "local", Local: true, Online: true, Version: version}}
	for _, h := range a.hosts.list() {
		hosts = append(hosts, hostInfo{Name: h.name, Online: true, Version: h.version})
	}
	writeJSON(w, http.StatusOK, hosts)
}

// hostContainers lists containers on the named host: the local docker client for
// "local", else an RPC to that agent.
func (a *api) hostContainers(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "host")
	if name == "" || name == "local" {
		out := []agentrpc.RemoteContainer{}
		if a.docker != nil {
			list, err := a.docker.ListContainers(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			for _, c := range list {
				if c.Oneoff {
					continue
				}
				out = append(out, agentrpc.RemoteContainer{
					Name: c.Name, Image: c.Image, State: c.State, Status: c.Status,
					Health: c.Health, Stack: c.Project, Service: c.Service,
				})
			}
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	h := a.hosts.get(name)
	if h == nil {
		writeError(w, http.StatusNotFound, "host not connected: "+name)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	resp, err := h.Call(ctx, agentrpc.MethodListContainers, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "remote host error: "+err.Error())
		return
	}
	if resp.Error != "" {
		writeError(w, http.StatusBadGateway, "remote host error: "+resp.Error)
		return
	}
	// resp.Result is already the JSON array the agent produced.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp.Result)
}

// bearerEquals compares an "Authorization: Bearer <token>" header to token in
// constant time.
func bearerEquals(header, token string) bool {
	const p = "Bearer "
	if !strings.HasPrefix(header, p) {
		return false
	}
	got := strings.TrimSpace(header[len(p):])
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

// sanitizeHostName reduces an agent-supplied name to a safe short identifier so
// it's a stable, injection-free map key and URL segment.
func sanitizeHostName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}
