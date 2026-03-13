package tmux

import (
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/mistakeknot/intermux/internal/activity"
	"gopkg.in/yaml.v3"
)

// defaultKeywords are the built-in agent type identifiers that appear in session names.
// Order matters — check compound keywords first ("admin-claude" before "claude").
var defaultKeywords = []string{
	"admin-claude",
	"claude",
	"codex",
	"dev",
}

// agentKeywords is the active keyword list — starts as defaultKeywords,
// extended by LoadKeywordsFromRegistry if a registry file is found.
var agentKeywords = append([]string{}, defaultKeywords...)

// registryFile is a minimal representation of fleet-registry.yaml for keyword extraction.
type registryFile struct {
	Agents map[string]registryAgent `yaml:"agents"`
}

type registryAgent struct {
	Runtime struct {
		Mode string `yaml:"mode"`
	} `yaml:"runtime"`
	Tags []string `yaml:"tags"`
}

// LoadKeywordsFromRegistry reads agent names from a fleet-registry.yaml file
// and adds them to the agent keywords list. Only agents with runtime.mode "cli"
// or tags containing "session" are added. Falls back silently to defaults if
// the file is missing or malformed.
func LoadKeywordsFromRegistry(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		slog.Debug("fleet registry not found, using default keywords", "path", path)
		return
	}

	var reg registryFile
	if err := yaml.Unmarshal(data, &reg); err != nil {
		slog.Warn("fleet registry parse error, using default keywords", "path", path, "err", err)
		return
	}

	// Collect agent names that represent interactive sessions.
	var extra []string
	for name, agent := range reg.Agents {
		if agent.Runtime.Mode == "cli" || containsTag(agent.Tags, "session") {
			// Skip names already in defaults.
			if !containsKeyword(defaultKeywords, name) {
				extra = append(extra, name)
			}
		}
	}

	if len(extra) == 0 {
		return
	}

	// Sort by segment count descending (compound keywords first), then alphabetically.
	sort.Slice(extra, func(i, j int) bool {
		ci := strings.Count(extra[i], "-") + 1
		cj := strings.Count(extra[j], "-") + 1
		if ci != cj {
			return ci > cj
		}
		return extra[i] < extra[j]
	})

	// Prepend extra (compound-first) before defaults for correct scan order.
	agentKeywords = append(extra, defaultKeywords...)
	slog.Info("loaded registry keywords", "count", len(extra), "total", len(agentKeywords))
}

func containsTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}

func containsKeyword(keywords []string, target string) bool {
	for _, kw := range keywords {
		if kw == target {
			return true
		}
	}
	return false
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
