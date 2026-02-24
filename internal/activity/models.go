package activity

import "time"

// AgentStatus describes the observed state of an agent.
type AgentStatus string

const (
	StatusActive  AgentStatus = "active"  // processing (spinner visible or recent output)
	StatusIdle    AgentStatus = "idle"    // waiting for user input (prompt visible)
	StatusStuck   AgentStatus = "stuck"   // no change for >5 minutes while supposedly active
	StatusCrashed AgentStatus = "crashed" // process dead or zombie
	StatusUnknown AgentStatus = "unknown" // can't determine
)

// AgentActivity represents the observed state of a single agent session.
type AgentActivity struct {
	AgentID      string            `json:"agent_id"`      // intermute agent ID (if known)
	TmuxSession  string            `json:"tmux_session"`  // tmux session name
	Terminal     string            `json:"terminal"`      // parsed: terminal app (e.g., "warp", "iterm", "ghostty")
	Project      string            `json:"project"`       // parsed: project name (e.g., "clavain", "shadow-work")
	AgentType    string            `json:"agent_type"`    // parsed: agent type (e.g., "claude", "codex", "dev")
	AgentNumber  int               `json:"agent_number"`  // parsed: instance number (0 if none)
	ProjectDir   string            `json:"project_dir"`   // resolved project directory from CWD (e.g., "/root/projects/Interverse/os/clavain")
	PID          int               `json:"pid"`           // Claude process PID
	CWD          string            `json:"cwd"`           // working directory
	GitBranch    string            `json:"git_branch"`    // current branch
	Status       AgentStatus       `json:"status"`        // active, idle, stuck, crashed, unknown
	LastOutput   string            `json:"last_output"`   // last meaningful line from pane
	ActiveBeads  []string          `json:"active_beads"`  // bead IDs (from pane content)
	FilesTouched []string          `json:"files_touched"` // recent files (from pane content)
	Metadata     map[string]string `json:"metadata"`      // arbitrary key-value pairs
	LastSeen     time.Time         `json:"last_seen"`     // last time we saw activity
	UpdatedAt    time.Time         `json:"updated_at"`    // last time this record was updated
}

// ParsedSessionName holds the components extracted from a tmux session name.
// Convention: {terminal}-{project}-{agent}-{optional_number}
// Examples:
//   "warp-clavain-claude-1"      → {warp, clavain, claude, 1}
//   "ghostty-shadow-work-codex"  → {ghostty, shadow-work, codex, 0}
//   "iterm-agent-fortress-claude" → {iterm, agent-fortress, claude, 0}
//   "main"                       → {main, "", "", 0}
type ParsedSessionName struct {
	Terminal    string
	Project     string
	AgentType   string
	AgentNumber int
	IsAgent     bool // true if this looks like an agent session
}

// ActivityEvent represents a single observed event.
type ActivityEvent struct {
	TmuxSession string            `json:"tmux_session"`
	Timestamp   time.Time         `json:"timestamp"`
	EventType   string            `json:"event_type"` // "file_edit", "command_run", "bead_update", "error", "idle", "session_start", "session_end"
	Summary     string            `json:"summary"`    // human-readable: "edited router.go", "ran go test"
	Details     map[string]string `json:"details"`
}

// SessionInfo holds raw tmux session information.
type SessionInfo struct {
	Name        string `json:"name"`
	Terminal    string `json:"terminal,omitempty"`
	Project     string `json:"project,omitempty"`
	AgentType   string `json:"agent_type,omitempty"`
	AgentNumber int    `json:"agent_number,omitempty"`
	IsAgent     bool   `json:"is_agent"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Created     string `json:"created"`
	Attached    bool   `json:"attached"`
	Windows     int    `json:"windows"`
	PanePID     int    `json:"pane_pid"`
}
