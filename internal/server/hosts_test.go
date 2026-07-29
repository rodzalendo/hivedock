package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/rogalinski/hivedock/internal/agentrpc"
)

// TestHostConnCall exercises the manager↔agent RPC correlation over a real
// WebSocket pair: the manager wraps its side in a hostConn+readLoop, a stand-in
// agent answers requests, and Call must return the matching reply.
func TestHostConnCall(t *testing.T) {
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	hcCh := make(chan *hostConn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		h := newHostConn("test", "1.0", conn)
		hcCh <- h
		h.readLoop() // returns when the client closes
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	cli, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cli.Close()

	// Stand-in agent: reply to listContainers, error on anything else.
	go func() {
		for {
			var req agentrpc.Request
			if err := cli.ReadJSON(&req); err != nil {
				return
			}
			resp := agentrpc.Response{ID: req.ID}
			if req.Method == agentrpc.MethodListContainers {
				b, _ := json.Marshal([]agentrpc.RemoteContainer{{Name: "web", State: "running"}})
				resp.Result = b
			} else {
				resp.Error = "unknown method"
			}
			_ = cli.WriteJSON(resp)
		}
	}()

	h := <-hcCh
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := h.Call(ctx, agentrpc.MethodListContainers, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected app error: %s", resp.Error)
	}
	var got []agentrpc.RemoteContainer
	if err := json.Unmarshal(resp.Result, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "web" {
		t.Fatalf("unexpected result: %+v", got)
	}

	// A second call still correlates to its own reply.
	if _, err := h.Call(ctx, agentrpc.MethodListContainers, nil); err != nil {
		t.Fatalf("second Call: %v", err)
	}
	// An agent-side error is carried in the Response (transport err stays nil).
	bogus, err := h.Call(ctx, "bogus", nil)
	if err != nil {
		t.Fatalf("transport error on unknown method: %v", err)
	}
	if bogus.Error == "" {
		t.Fatal("expected an application error for the unknown method")
	}
}

// TestHostConnCallStream exercises streaming frames + cancellation: chunks then a
// terminal (channel closes), and a cancelled ctx propagates a `cancel` frame to
// the agent which ends the stream.
func TestHostConnCallStream(t *testing.T) {
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	hcCh := make(chan *hostConn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		h := newHostConn("t", "1.0", conn)
		hcCh <- h
		h.readLoop()
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	cli, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cli.Close()

	// Stand-in agent: single writer draining `out`; a reader that emits streams.
	out := make(chan agentrpc.Response, 64)
	go func() {
		for m := range out {
			if cli.WriteJSON(m) != nil {
				return
			}
		}
	}()
	gotCancel := make(chan string, 4)
	go func() {
		stops := map[string]chan struct{}{}
		for {
			var req agentrpc.Request
			if err := cli.ReadJSON(&req); err != nil {
				return
			}
			switch req.Method {
			case agentrpc.MethodCancel:
				var p agentrpc.CancelParams
				_ = json.Unmarshal(req.Params, &p)
				gotCancel <- p.ID
				if st := stops[p.ID]; st != nil {
					close(st)
					delete(stops, p.ID)
				}
			case "streamN":
				for i := 0; i < 3; i++ {
					b, _ := json.Marshal(fmt.Sprintf("line%d", i))
					out <- agentrpc.Response{ID: req.ID, Kind: agentrpc.KindStream, Result: b}
				}
				out <- agentrpc.Response{ID: req.ID} // terminal ok
			case "streamForever":
				id := req.ID
				st := make(chan struct{})
				stops[id] = st
				go func() {
					for {
						select {
						case <-st:
							out <- agentrpc.Response{ID: id, Error: "canceled", Code: "canceled"}
							return
						default:
							out <- agentrpc.Response{ID: id, Kind: agentrpc.KindStream, Result: json.RawMessage(`"x"`)}
							time.Sleep(3 * time.Millisecond)
						}
					}
				}()
			}
		}
	}()

	h := <-hcCh

	// Basic streaming: 3 chunks + terminal, then the channel closes.
	ch, _, err := h.CallStream(context.Background(), "streamN", nil)
	if err != nil {
		t.Fatalf("CallStream: %v", err)
	}
	var chunks []string
	var sawTerminal bool
	for f := range ch {
		if f.Kind == agentrpc.KindStream {
			var s string
			_ = json.Unmarshal(f.Result, &s)
			chunks = append(chunks, s)
		} else {
			sawTerminal = true
		}
	}
	if len(chunks) != 3 || !sawTerminal {
		t.Fatalf("stream = %d chunks, terminal=%v (want 3, true)", len(chunks), sawTerminal)
	}

	// Cancellation: cancelling ctx sends `cancel` with the call id; the agent ends
	// the stream and the channel closes.
	ctx, cancel := context.WithCancel(context.Background())
	ch2, id2, err := h.CallStream(ctx, "streamForever", nil)
	if err != nil {
		t.Fatalf("CallStream2: %v", err)
	}
	<-ch2 // receive at least one chunk
	cancel()
	select {
	case gotID := <-gotCancel:
		if gotID != id2 {
			t.Errorf("cancel id = %q, want %q", gotID, id2)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("agent never received cancel")
	}
	// Channel must eventually close.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch2:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("stream did not close after cancel")
		}
	}
}

func TestBearerEquals(t *testing.T) {
	cases := []struct {
		header, token string
		want          bool
	}{
		{"Bearer secret", "secret", true},
		{"Bearer secret", "other", false},
		{"Bearer  secret ", "secret", true}, // trimmed
		{"secret", "secret", false},         // missing prefix
		{"", "secret", false},
		{"Bearer ", "", true}, // empty token matches empty (endpoint is disabled elsewhere)
	}
	for _, c := range cases {
		if got := bearerEquals(c.header, c.token); got != c.want {
			t.Errorf("bearerEquals(%q, %q) = %v, want %v", c.header, c.token, got, c.want)
		}
	}
}

func TestSanitizeHostName(t *testing.T) {
	cases := map[string]string{
		"Home Server!":          "homeserver",
		"  VPS-1  ":             "vps-1",
		"a/b/../c":              "abc",
		"MixedCASE_123":         "mixedcase_123",
		strings.Repeat("x", 50): strings.Repeat("x", 32),
		"":                      "",
	}
	for in, want := range cases {
		if got := sanitizeHostName(in); got != want {
			t.Errorf("sanitizeHostName(%q) = %q, want %q", in, got, want)
		}
	}
}
