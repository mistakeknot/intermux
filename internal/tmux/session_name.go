package tmux

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/mistakeknot/intermux/internal/activity"
)

// bracketSessionRe matches the iTerm/rio launcher convention:
//
//	{terminal}[{project}(@{agent})? - {session_uuid}
//
// with either "[" or "]" separating terminal from project, e.g.
//
//	"iterm[jeddnet@codex - 019f805d-303f-7c43-a79e-7e1893411b25"
//	"iterm]jawnomicon - d58d5e63-b647-4d68-82f8-64d787310d15"
//	"rio[clavain - aa2bb078-ee16-4c32-9f97-01ef7dbdec61"
//
// The UUID tail is the agent's own session ID; without an @marker the
// agent is Claude Code, since that launcher only annotates non-default
// agents (e.g. "@codex").
var bracketSessionRe = regexp.MustCompile(
	`^([^\[\]]+)[\[\]](.+?)\s+-\s+([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

// defaultKeywords are the built-in agent type identifiers that appear in session names.
// Order matters — check compound keywords first ("admin-claude" before "claude").
var defaultKeywords = []string{
	"admin-claude",
	"claude",
	"codex",
	"dev",
}

// agentKeywords is the active keyword list.
var agentKeywords = append([]string{}, defaultKeywords...)

// ParseSessionName extracts terminal, project, agent type, and instance number
// from a tmux session name. Two conventions are recognized:
//
//	{terminal}[{project}(@{agent})? - {session_uuid}   (iTerm/rio launcher; "]" also accepted)
//	{terminal}-{project}-{agent}-{optional_number}     (legacy fleet convention)
//
// In the legacy form the project component can contain hyphens (e.g.,
// "shadow-work", "agent-fortress"), so the parser scans for the rightmost
// known agent keyword.
func ParseSessionName(name string) activity.ParsedSessionName {
	result := activity.ParsedSessionName{
		Terminal: name, // fallback: whole name is "terminal"
	}

	if m := bracketSessionRe.FindStringSubmatch(name); m != nil {
		result.Terminal = strings.TrimSpace(m[1])
		body := strings.TrimSpace(m[2])
		result.AgentType = "claude"
		if at := strings.LastIndex(body, "@"); at >= 0 {
			if agent := strings.TrimSpace(body[at+1:]); agent != "" {
				result.AgentType = strings.ToLower(agent)
				body = strings.TrimSpace(body[:at])
			}
		}
		result.Project = body
		result.IsAgent = true
		return result
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
	result.Project = strings.Join(rest, "-")
	return result
}

// IsAgentSession returns true if the session name matches the agent naming convention.
func IsAgentSession(name string) bool {
	return ParseSessionName(name).IsAgent
}
