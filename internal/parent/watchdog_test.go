package parent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestWatch_FiresCallbackOnInitParent verifies that ppid==1 causes
// the watchdog to invoke onParentDeath.
func TestWatch_FiresCallbackOnInitParent(t *testing.T) {
	orig := getppid
	t.Cleanup(func() { getppid = orig })
	getppid = func() int { return 1 }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fired := make(chan struct{})
	done := make(chan struct{})

	go func() {
		Watch(ctx, 10*time.Millisecond, func() { close(fired) })
		close(done)
	}()

	select {
	case <-fired:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("onParentDeath did not fire within 500ms")
	}

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("watchdog goroutine did not exit after firing callback")
	}
}

// TestWatch_DoesNotFireWhileParentAlive verifies that a non-init ppid
// does not invoke the callback.
func TestWatch_DoesNotFireWhileParentAlive(t *testing.T) {
	orig := getppid
	t.Cleanup(func() { getppid = orig })

	var calls atomic.Int32
	getppid = func() int {
		calls.Add(1)
		return 12345 // any non-init pid
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	var fired atomic.Bool
	go func() {
		Watch(ctx, 5*time.Millisecond, func() { fired.Store(true) })
		close(done)
	}()

	// Give it a few ticks to confirm it doesn't fire.
	time.Sleep(50 * time.Millisecond)

	if fired.Load() {
		t.Fatal("onParentDeath fired despite ppid being alive")
	}
	if calls.Load() < 2 {
		t.Fatalf("expected getppid to be polled at least twice, got %d", calls.Load())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("watchdog goroutine did not exit after explicit cancel")
	}
}

// TestWatch_ExitsOnContextCancel verifies the watchdog respects ctx.Done()
// when the parent is still alive.
func TestWatch_ExitsOnContextCancel(t *testing.T) {
	orig := getppid
	t.Cleanup(func() { getppid = orig })
	getppid = func() int { return 12345 }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		Watch(ctx, 50*time.Millisecond, func() { t.Error("callback should not fire on cancel") })
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("watchdog did not exit on context cancel")
	}
}
