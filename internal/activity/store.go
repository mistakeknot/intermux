package activity

import (
	"strings"
	"sync"
	"time"
)

const defaultRingSize = 200

// Store holds agent activity data and an event ring buffer.
// Thread-safe via RWMutex.
type Store struct {
	mu        sync.RWMutex
	agents    map[string]*AgentActivity // keyed by tmux session name
	events    []ActivityEvent
	eventHead int // next write position in ring buffer
	eventFull bool
	ringSize  int
}

// NewStore creates an activity store with the given ring buffer size.
func NewStore(ringSize int) *Store {
	if ringSize <= 0 {
		ringSize = defaultRingSize
	}
	return &Store{
		agents:   make(map[string]*AgentActivity),
		events:   make([]ActivityEvent, ringSize),
		ringSize: ringSize,
	}
}

// Update sets the activity record for a tmux session.
func (s *Store) Update(session string, activity AgentActivity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	activity.UpdatedAt = time.Now()
	s.agents[session] = &activity
}

// Get returns the activity for a specific session.
func (s *Store) Get(session string) *AgentActivity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[session]
	if !ok {
		return nil
	}
	copy := *a
	return &copy
}

// List returns all current agent activities.
func (s *Store) List() []AgentActivity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentActivity, 0, len(s.agents))
	for _, a := range s.agents {
		out = append(out, *a)
	}
	return out
}

// Remove deletes the activity record for a session (e.g., when session ends).
func (s *Store) Remove(session string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.agents, session)
}

// PushEvent adds an event to the ring buffer.
func (s *Store) PushEvent(ev ActivityEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[s.eventHead] = ev
	s.eventHead = (s.eventHead + 1) % s.ringSize
	if s.eventHead == 0 {
		s.eventFull = true
	}
}

// Feed returns events since the given time, optionally filtered by session and/or event type.
func (s *Store) Feed(since time.Time, session, eventType string) []ActivityEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var all []ActivityEvent
	if s.eventFull {
		// Read from head to end, then 0 to head
		for i := s.eventHead; i < s.ringSize; i++ {
			all = append(all, s.events[i])
		}
		for i := 0; i < s.eventHead; i++ {
			all = append(all, s.events[i])
		}
	} else {
		all = s.events[:s.eventHead]
	}

	var out []ActivityEvent
	for _, ev := range all {
		if ev.Timestamp.IsZero() {
			continue
		}
		if ev.Timestamp.Before(since) {
			continue
		}
		if session != "" && ev.TmuxSession != session {
			continue
		}
		if eventType != "" && ev.EventType != eventType {
			continue
		}
		out = append(out, ev)
	}
	return out
}

// Search returns events whose summary contains the query string.
func (s *Store) Search(query string) []ActivityEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	q := strings.ToLower(query)
	var out []ActivityEvent
	count := s.ringSize
	if !s.eventFull {
		count = s.eventHead
	}
	for i := 0; i < count; i++ {
		ev := s.events[i]
		if ev.Timestamp.IsZero() {
			continue
		}
		if strings.Contains(strings.ToLower(ev.Summary), q) {
			out = append(out, ev)
		}
	}
	return out
}

// SetAgentCorrelation updates the intermute agent ID for a tmux session.
func (s *Store) SetAgentCorrelation(tmuxSession, agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.agents[tmuxSession]
	if ok {
		a.AgentID = agentID
	}
}
