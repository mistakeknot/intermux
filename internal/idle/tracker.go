// Package idle provides a shared idle-state tracker for intermux background goroutines.
//
// When no MCP tool calls arrive for a configurable grace period, the tracker
// transitions to "idle" and signals background goroutines to back off their
// tick rates. A wake channel lets goroutines immediately resume normal rates
// when a request arrives.
package idle

import (
	"sync"
	"time"
)

// Tracker monitors MCP request activity and exposes idle state to background
// goroutines so they can reduce CPU usage when no client is actively using
// the server.
type Tracker struct {
	mu        sync.Mutex
	lastTouch time.Time
	grace     time.Duration // idle after this long with no Touch()
	wakeCh    chan struct{} // closed+replaced on each Touch while idle
}

// NewTracker creates a tracker with the given grace period.
// Background goroutines are considered idle when no Touch() has been
// called for longer than grace.
func NewTracker(grace time.Duration) *Tracker {
	return &Tracker{
		lastTouch: time.Now(),
		grace:     grace,
		wakeCh:    make(chan struct{}),
	}
}

// Touch records that an MCP request was just received.
// If the tracker was idle, it wakes all waiting goroutines.
func (t *Tracker) Touch() {
	t.mu.Lock()
	defer t.mu.Unlock()
	wasIdle := time.Since(t.lastTouch) > t.grace
	t.lastTouch = time.Now()
	if wasIdle {
		// Wake all goroutines blocked in WaitForWake
		close(t.wakeCh)
		t.wakeCh = make(chan struct{})
	}
}

// IsIdle returns true if no Touch has occurred within the grace period.
func (t *Tracker) IsIdle() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return time.Since(t.lastTouch) > t.grace
}

// WakeCh returns a channel that is closed when Touch() is called while idle.
// Callers should select on this alongside their ticker to immediately resume
// work when the server becomes active again.
//
// The returned channel is replaced each time it fires, so callers must
// re-fetch it on each loop iteration.
func (t *Tracker) WakeCh() <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.wakeCh
}
