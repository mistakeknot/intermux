package health

import (
	"context"
	"log"
	"time"

	"github.com/mistakeknot/intermux/internal/activity"
	"github.com/mistakeknot/intermux/internal/idle"
)

// Report contains health status for a single agent.
type Report struct {
	TmuxSession string                `json:"tmux_session"`
	AgentID     string                `json:"agent_id,omitempty"`
	Status      activity.AgentStatus  `json:"status"`
	LastSeen    time.Time             `json:"last_seen"`
	IdleSince   *time.Time            `json:"idle_since,omitempty"`
	Warnings    []string              `json:"warnings,omitempty"`
}

// MonitorConfig configures the health monitor.
type MonitorConfig struct {
	Interval     time.Duration // check interval (default 30s)
	StuckTimeout time.Duration // consider stuck after this duration (default 5m)
	CrashCheck   bool          // check if process PIDs are alive
}

// DefaultMonitorConfig returns sensible defaults.
func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		Interval:     30 * time.Second,
		StuckTimeout: 5 * time.Minute,
		CrashCheck:   true,
	}
}

// idleCheckInterval is the health check interval when the server is idle.
const idleCheckInterval = 5 * time.Minute

// Monitor watches agent health and classifies status.
type Monitor struct {
	config      MonitorConfig
	store       *activity.Store
	idleTracker *idle.Tracker
	// Callback for status changes (e.g., push to intermute)
	OnStatusChange func(session string, old, new activity.AgentStatus)
}

// NewMonitor creates a health monitor.
func NewMonitor(config MonitorConfig, store *activity.Store) *Monitor {
	return &Monitor{
		config: config,
		store:  store,
	}
}

// SetIdleTracker attaches an idle tracker for adaptive tick rates.
func (m *Monitor) SetIdleTracker(t *idle.Tracker) {
	m.idleTracker = t
}

// Run starts the monitor loop. Blocks until context is cancelled.
// When an idle tracker is attached, backs off to idleCheckInterval when
// no MCP traffic has been seen.
func (m *Monitor) Run(ctx context.Context) {
	log.Printf("intermux: health monitor started (interval=%s)", m.config.Interval)

	activeTicker := time.NewTicker(m.config.Interval)
	idleTicker := time.NewTicker(idleCheckInterval)
	defer activeTicker.Stop()
	defer idleTicker.Stop()

	for {
		if m.idleTracker != nil && m.idleTracker.IsIdle() {
			select {
			case <-ctx.Done():
				log.Printf("intermux: health monitor stopped")
				return
			case <-idleTicker.C:
				m.check()
			case <-m.idleTracker.WakeCh():
				m.check()
				activeTicker.Reset(m.config.Interval)
			}
		} else {
			select {
			case <-ctx.Done():
				log.Printf("intermux: health monitor stopped")
				return
			case <-activeTicker.C:
				m.check()
			}
		}
	}
}

func (m *Monitor) check() {
	agents := m.store.List()
	for _, agent := range agents {
		var warnings []string

		// Check for stale agents
		if time.Since(agent.LastSeen) > m.config.StuckTimeout {
			if agent.Status == activity.StatusActive {
				if m.OnStatusChange != nil {
					m.OnStatusChange(agent.TmuxSession, agent.Status, activity.StatusStuck)
				}
				agent.Status = activity.StatusStuck
				warnings = append(warnings, "no activity for >5 minutes while supposedly active")
				m.store.Update(agent.TmuxSession, agent)
			}
		}

		if len(warnings) > 0 {
			log.Printf("intermux: health warning for %s: %v", agent.TmuxSession, warnings)
		}
	}
}

// Report generates health reports for all known agents.
func (m *Monitor) Report() []Report {
	agents := m.store.List()
	reports := make([]Report, 0, len(agents))
	for _, agent := range agents {
		r := Report{
			TmuxSession: agent.TmuxSession,
			AgentID:     agent.AgentID,
			Status:      agent.Status,
			LastSeen:     agent.LastSeen,
		}

		// Add warnings
		if agent.Status == activity.StatusStuck {
			r.Warnings = append(r.Warnings, "agent appears stuck — no output change for >5 minutes")
		}
		if agent.Status == activity.StatusCrashed {
			r.Warnings = append(r.Warnings, "agent process not found — may have crashed")
		}

		reports = append(reports, r)
	}
	return reports
}
