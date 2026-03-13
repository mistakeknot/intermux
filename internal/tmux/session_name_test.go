package tmux

import (
	"os"
	"path/filepath"
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

func TestLoadKeywordsFromRegistry(t *testing.T) {
	// Reset keywords before each subtest.
	resetKeywords := func() {
		agentKeywords = append([]string{}, defaultKeywords...)
	}

	t.Run("missing file falls back to defaults", func(t *testing.T) {
		resetKeywords()
		LoadKeywordsFromRegistry("/nonexistent/path.yaml")
		if len(agentKeywords) != len(defaultKeywords) {
			t.Errorf("expected %d keywords, got %d", len(defaultKeywords), len(agentKeywords))
		}
	})

	t.Run("malformed YAML falls back to defaults", func(t *testing.T) {
		resetKeywords()
		tmp := filepath.Join(t.TempDir(), "bad.yaml")
		os.WriteFile(tmp, []byte("not: [valid: yaml: {{"), 0644)
		LoadKeywordsFromRegistry(tmp)
		if len(agentKeywords) != len(defaultKeywords) {
			t.Errorf("expected %d keywords, got %d", len(defaultKeywords), len(agentKeywords))
		}
	})

	t.Run("loads CLI agents from registry", func(t *testing.T) {
		resetKeywords()
		yaml := `
agents:
  grey-area:
    runtime:
      mode: cli
    tags: [orchestration]
  falling-outside:
    runtime:
      mode: cli
    tags: [orchestration]
  mistake-not:
    runtime:
      mode: cli
    tags: []
  fd-architecture:
    runtime:
      mode: subagent
    tags: [review]
`
		tmp := filepath.Join(t.TempDir(), "registry.yaml")
		os.WriteFile(tmp, []byte(yaml), 0644)
		LoadKeywordsFromRegistry(tmp)

		// Should have 3 new + 4 defaults = 7
		if len(agentKeywords) != 7 {
			t.Fatalf("expected 7 keywords, got %d: %v", len(agentKeywords), agentKeywords)
		}

		// Verify Culture ship names are recognized
		got := ParseSessionName("iterm-Demarch-grey-area-01")
		if !got.IsAgent {
			t.Error("grey-area not recognized as agent")
		}
		if got.AgentType != "grey-area" {
			t.Errorf("AgentType: got %q, want %q", got.AgentType, "grey-area")
		}
		if got.Project != "demarch" && got.Project != "Demarch" {
			t.Errorf("Project: got %q, want Demarch", got.Project)
		}

		got2 := ParseSessionName("iterm-Demarch-falling-outside-01")
		if !got2.IsAgent {
			t.Error("falling-outside not recognized as agent")
		}
		if got2.AgentType != "falling-outside" {
			t.Errorf("AgentType: got %q, want %q", got2.AgentType, "falling-outside")
		}

		// Subagent-mode agents should NOT be added
		got3 := ParseSessionName("iterm-Demarch-fd-architecture")
		if got3.IsAgent {
			t.Error("fd-architecture (subagent mode) should not be recognized as agent keyword")
		}
	})

	t.Run("session-tagged agents are loaded", func(t *testing.T) {
		resetKeywords()
		yaml := `
agents:
  sleeper-service:
    runtime:
      mode: daemon
    tags: [session]
`
		tmp := filepath.Join(t.TempDir(), "registry.yaml")
		os.WriteFile(tmp, []byte(yaml), 0644)
		LoadKeywordsFromRegistry(tmp)

		if len(agentKeywords) != 5 { // 1 new + 4 defaults
			t.Fatalf("expected 5 keywords, got %d", len(agentKeywords))
		}

		got := ParseSessionName("rio-Demarch-sleeper-service")
		if !got.IsAgent {
			t.Error("sleeper-service not recognized as agent")
		}
	})

	// Reset for subsequent tests
	resetKeywords()
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
