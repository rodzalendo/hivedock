package events

import (
	"testing"
	"time"
)

func TestHubDebouncesAndDelivers(t *testing.T) {
	h := NewHub(20 * time.Millisecond)
	ch, unsub := h.Subscribe()
	defer unsub()

	// A burst of notifications must coalesce into a single delivered message.
	for i := 0; i < 10; i++ {
		h.NotifyChanged("fs")
	}

	select {
	case msg := <-ch:
		if msg.Type != "stacks:changed" {
			t.Fatalf("type = %q, want stacks:changed", msg.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("expected a broadcast within 1s")
	}

	// No second message should arrive from the same burst.
	select {
	case msg := <-ch:
		t.Fatalf("unexpected second message: %+v", msg)
	case <-time.After(80 * time.Millisecond):
	}
}

func TestHubUnsubscribeStopsDelivery(t *testing.T) {
	h := NewHub(5 * time.Millisecond)
	ch, unsub := h.Subscribe()
	unsub()

	// Channel is closed on unsubscribe.
	if _, ok := <-ch; ok {
		t.Fatal("expected channel closed after unsubscribe")
	}

	// Notifying after unsubscribe must not panic.
	h.NotifyChanged("fs")
	time.Sleep(20 * time.Millisecond)
}
