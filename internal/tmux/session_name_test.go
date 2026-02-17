package tmux

import (
	"testing"
)

func TestParseSessionName(t *testing.T) {
	tests := []struct {
		name        string
		wantTerm    string
		wantProject string
		wantAgent   string
		wantNum     int
		wantIsAgent bool
	}{
		// Standard: terminal-project-agent
		{"warp-clavain-claude", "warp", "clavain", "claude", 0, true},
		{"warp-intermute-claude", "warp", "intermute", "claude", 0, true},
		{"rio-interdoc-claude", "rio", "interdoc", "claude", 0, true},
		{"iterm-pmos-claude", "iterm", "pmos", "claude", 0, true},

		// With instance number: terminal-project-agent-N
		{"warp-clavain-claude-1", "warp", "clavain", "claude", 1, true},
		{"warp-clavain-claude-2", "warp", "clavain", "claude", 2, true},
		{"rio-autarch-claude-3", "rio", "autarch", "claude", 3, true},
		{"alacritty-agmodb-claude-1", "alacritty", "agmodb", "claude", 1, true},

		// Codex agents
		{"alacritty-agmodb-codex", "alacritty", "agmodb", "codex", 0, true},
		{"iterm-agentmud-codex", "iterm", "agentmud", "codex", 0, true},
		{"warp-interverse-codex", "warp", "interverse", "codex", 0, true},
		{"wezterm-typhon-codex", "wezterm", "typhon", "codex", 0, true},

		// Codex with numbers
		{"ghostty-shadow-work-codex-1", "ghostty", "shadow-work", "codex", 1, true},
		{"ghostty-shadow-work-codex-4", "ghostty", "shadow-work", "codex", 4, true},
		{"kitty-ong-lots-codex-1", "kitty", "ong-lots", "codex", 1, true},

		// Multi-word projects
		{"iterm-agent-fortress-claude", "iterm", "agent-fortress", "claude", 0, true},
		{"iterm-agent-fortress-codex", "iterm", "agent-fortress", "codex", 0, true},
		{"warp-agent-rig-claude", "warp", "agent-rig", "claude", 0, true},

		// Dev agent type
		{"rio-autarch-dev", "rio", "autarch", "dev", 0, true},

		// Admin-claude compound keyword
		{"rio-interverse-admin-claude", "rio", "interverse", "admin-claude", 0, true},

		// Two-word project + claude + number
		{"wezterm-typhon-claude-2", "wezterm", "typhon", "claude", 2, true},

		// Non-agent sessions
		{"main", "main", "", "", 0, false},

		// Edge case: terminal-claude-root (claude is the project? or agent?)
		// With the parser, "root" isn't a known agent type, so "claude-root" becomes project.
		// But "claude" IS a keyword — so it should parse as terminal=terminal, project="", agent=claude
		// with "root" being stripped... Actually let's trace this:
		// parts = ["terminal", "claude", "root"]
		// terminal = "terminal", rest = ["claude", "root"]
		// last segment "root" is not a number, so rest stays ["claude", "root"]
		// scan for agent: "root" isn't an agent. "claude" at rest[0] is, but we scan from the right.
		// Actually we check rest[len(rest)-1] = "root" — not a keyword.
		// Then we check rest[len(rest)-2:] = "claude-root" — not a keyword.
		// So it falls through: project = "claude-root", not an agent session.
		{"terminal-claude-root", "terminal", "claude-root", "", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSessionName(tt.name)
			if got.Terminal != tt.wantTerm {
				t.Errorf("Terminal: got %q, want %q", got.Terminal, tt.wantTerm)
			}
			if got.Project != tt.wantProject {
				t.Errorf("Project: got %q, want %q", got.Project, tt.wantProject)
			}
			if got.AgentType != tt.wantAgent {
				t.Errorf("AgentType: got %q, want %q", got.AgentType, tt.wantAgent)
			}
			if got.AgentNumber != tt.wantNum {
				t.Errorf("AgentNumber: got %d, want %d", got.AgentNumber, tt.wantNum)
			}
			if got.IsAgent != tt.wantIsAgent {
				t.Errorf("IsAgent: got %v, want %v", got.IsAgent, tt.wantIsAgent)
			}
		})
	}
}

func TestIsAgentSession(t *testing.T) {
	agents := []string{
		"warp-clavain-claude-1",
		"ghostty-shadow-work-codex-2",
		"iterm-agent-fortress-claude",
		"rio-autarch-dev",
	}
	for _, name := range agents {
		if !IsAgentSession(name) {
			t.Errorf("expected %q to be an agent session", name)
		}
	}

	nonAgents := []string{
		"main",
		"terminal-claude-root",
	}
	for _, name := range nonAgents {
		if IsAgentSession(name) {
			t.Errorf("expected %q to NOT be an agent session", name)
		}
	}
}
