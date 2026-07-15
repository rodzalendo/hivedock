package server

import (
	"testing"
	"time"
)

func TestLoginLimiter(t *testing.T) {
	l := newLoginLimiter()
	key := "login:admin|1.2.3.4"

	// Below the threshold: failures accrue but never block.
	for i := 0; i < loginFailThreshold-1; i++ {
		if d := l.fail(key); d != 0 {
			t.Fatalf("fail #%d blocked early (%v)", i+1, d)
		}
		if l.retryAfter(key) != 0 {
			t.Fatalf("retryAfter after fail #%d should be 0", i+1)
		}
	}

	// The threshold failure starts the backoff.
	if d := l.fail(key); d <= 0 {
		t.Fatalf("threshold failure should block, got %v", d)
	}
	if l.retryAfter(key) <= 0 {
		t.Fatal("retryAfter should be positive while blocked")
	}

	// The first block is the base backoff (within a small margin).
	if got := l.retryAfter(key); got > loginBaseBackoff+time.Second {
		t.Fatalf("first block = %v, want ~%v", got, loginBaseBackoff)
	}

	// Backoff is capped.
	for i := 0; i < 40; i++ {
		l.fail(key)
	}
	if got := l.retryAfter(key); got > loginMaxBackoff+time.Second {
		t.Fatalf("block %v exceeds cap %v", got, loginMaxBackoff)
	}

	// reset clears the block.
	l.reset(key)
	if l.retryAfter(key) != 0 {
		t.Fatal("retryAfter should be 0 after reset")
	}

	// Distinct keys are independent.
	other := "login:admin|9.9.9.9"
	if l.retryAfter(other) != 0 {
		t.Fatal("unrelated key should not be blocked")
	}
}
