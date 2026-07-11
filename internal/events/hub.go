// Package events provides a small pub/sub hub that fans server-side change
// notifications out to connected WebSocket clients. Phase 1 uses a single topic
// ("state changed → refetch"); the protocol can grow message types later.
package events

import (
	"encoding/json"
	"sync"
	"time"
)

// Message is one event pushed to clients.
type Message struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

// Hub tracks subscribers and broadcasts messages to them. Safe for concurrent
// use. Broadcasts are coalesced (debounced) so a burst of filesystem/daemon
// events — e.g. `compose up` touching many containers — yields one client
// refresh, not dozens.
type Hub struct {
	mu       sync.Mutex
	subs     map[int]chan Message
	nextID   int
	debounce time.Duration

	pendMu  sync.Mutex
	pending map[string]Message
	timer   *time.Timer
}

// NewHub creates a hub. debounce is the coalescing window for NotifyChanged.
func NewHub(debounce time.Duration) *Hub {
	return &Hub{
		subs:     map[int]chan Message{},
		debounce: debounce,
		pending:  map[string]Message{},
	}
}

// Subscribe registers a client and returns its channel plus an unsubscribe func.
// The channel is buffered; a slow client that fills its buffer drops messages
// rather than blocking the hub (it will refetch on the next event anyway).
func (h *Hub) Subscribe() (<-chan Message, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := h.nextID
	h.nextID++
	ch := make(chan Message, 8)
	h.subs[id] = ch
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if c, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(c)
		}
	}
}

// broadcast sends immediately to all subscribers (non-blocking per client).
func (h *Hub) broadcast(msg Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs {
		select {
		case ch <- msg:
		default: // client buffer full; drop — it refetches on the next event
		}
	}
}

// Publish broadcasts a message to all subscribers immediately (no debouncing).
// Used for ordered, real-time streams like deploy:* output where every line
// matters and coalescing would drop content.
func (h *Hub) Publish(msg Message) {
	h.broadcast(msg)
}

// NotifyChanged schedules a coalesced broadcast of a change event. Multiple
// calls with the same reason within the debounce window collapse into one.
func (h *Hub) NotifyChanged(reason string) {
	h.pendMu.Lock()
	defer h.pendMu.Unlock()
	h.pending["stacks:changed"] = Message{Type: "stacks:changed", Payload: map[string]string{"reason": reason}}
	if h.timer == nil {
		h.timer = time.AfterFunc(h.debounce, h.flush)
	} else {
		h.timer.Reset(h.debounce)
	}
}

func (h *Hub) flush() {
	h.pendMu.Lock()
	msgs := make([]Message, 0, len(h.pending))
	for k, m := range h.pending {
		msgs = append(msgs, m)
		delete(h.pending, k)
	}
	h.timer = nil
	h.pendMu.Unlock()

	for _, m := range msgs {
		h.broadcast(m)
	}
}

// Encode marshals a message to JSON (helper for the WS handler).
func (m Message) Encode() ([]byte, error) { return json.Marshal(m) }
