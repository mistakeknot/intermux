package tmux

import (
	"strconv"
	"strings"

	"github.com/mistakeknot/intermux/internal/activity"
)

// agentKeywords are the known agent type identifiers that appear in session names.
// Order matters — check compound keywords first ("admin-claude" before "claude").
var agentKeywords = []string{
	"admin-claude",
	"claude",
	"codex",
	"dev",
}

// ParseSessionName extracts terminal, project, agent type, and instance number
// from a tmux session name following the convention:
//
//	{terminal}-{project}-{agent}-{optional_number}
//
// The project component can contain hyphens (e.g., "shadow-work", "agent-fortress"),
// so the parser scans for the rightmost known agent keyword.
func ParseSessionName(name string) activity.ParsedSessionName {
	result := activity.ParsedSessionName{
		Terminal: name, // fallback: whole name is "terminal"
	}

	parts := strings.Split(name, "-")
	if len(parts) < 2 {
		// Single word like "main" — not an agent session
		return result
	}

	// Terminal is always the first segment
	result.Terminal = parts[0]
	rest := parts[1:]

	// Check if the last segment is a number (agent instance)
	lastIdx := len(rest) - 1
	if num, err := strconv.Atoi(rest[lastIdx]); err == nil && num > 0 {
		result.AgentNumber = num
		rest = rest[:lastIdx]
	}

	if len(rest) == 0 {
		return result
	}

	// Scan from the right for a known agent keyword.
	// We need to handle compound keywords like "admin-claude" (two segments).
	restStr := strings.Join(rest, "-")
	for _, kw := range agentKeywords {
		kwParts := len(strings.Split(kw, "-"))
		// Check if the last kwParts segments of rest match this keyword
		if len(rest) >= kwParts {
			candidate := strings.Join(rest[len(rest)-kwParts:], "-")
			if strings.EqualFold(candidate, kw) {
				result.AgentType = strings.ToLower(candidate)
				result.IsAgent = true
				projectParts := rest[:len(rest)-kwParts]
				if len(projectParts) > 0 {
					result.Project = strings.Join(projectParts, "-")
				}
				return result
			}
		}
	}

	// No agent keyword found — treat the whole rest as project
	// This covers sessions like "rio-autarch-dev" where "dev" is in agentKeywords,
	// but it should already be caught above. For truly unrecognized patterns,
	// just store what we have.
	_ = restStr
	result.Project = strings.Join(rest, "-")
	return result
}

// IsAgentSession returns true if the session name matches the agent naming convention.
func IsAgentSession(name string) bool {
	return ParseSessionName(name).IsAgent
}
