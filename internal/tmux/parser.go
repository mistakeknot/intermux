package tmux

import (
	"regexp"
	"strings"

	"github.com/mistakeknot/intermux/internal/activity"
)

var (
	// Error patterns that suggest the agent is in trouble.
	errorPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)EACCES|permission denied`),
		regexp.MustCompile(`(?i)panic:`),
		regexp.MustCompile(`(?i)SIGKILL|SIGTERM|killed`),
		regexp.MustCompile(`(?i)out of memory|OOM`),
		regexp.MustCompile(`(?i)fatal error`),
		regexp.MustCompile(`(?i)ENOMEM`),
	}

	// Patterns for file edits (Claude Code output).
	fileEditPattern = regexp.MustCompile(`(?:Edit|Write|Read)\s+(/[^\s]+)`)

	// Bead activity patterns.
	// Bead IDs follow the format: {project}-{4-char alphanumeric}[.N[.N...]]
	// Examples: Sylveste-a1b2, Demarch-og7m, Sylveste-rsj.1.4
	beadPattern = regexp.MustCompile(`\b([A-Z][a-z]+-[a-z0-9]{3,5}(?:\.[0-9]+)*)\b`)

	// Git activity patterns.
	gitCommitPattern = regexp.MustCompile(`(?i)git commit|committed|create mode`)
	gitPushPattern   = regexp.MustCompile(`(?i)git push|Enumerating objects|Writing objects`)

	// Test activity patterns.
	testRunPattern = regexp.MustCompile(`(?i)go test|pytest|npm test|PASS|FAIL|--- PASS|--- FAIL`)

	// Claude Code spinner/activity indicators.
	activeIndicators = []string{
		"Thinking",
		"Reading",
		"Writing",
		"Editing",
		"Running",
		"Searching",
		"Analyzing",
	}

	// Prompt patterns that indicate the agent is idle/waiting.
	promptPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\$\s*$`),
		regexp.MustCompile(`>\s*$`),
		regexp.MustCompile(`claude>\s*$`),
		regexp.MustCompile(`\?\s+$`), // Claude Code question prompt
	}
)

// ParsedContent holds signals extracted from tmux pane content.
type ParsedContent struct {
	Status       activity.AgentStatus
	LastOutput   string
	FilesTouched []string
	ActiveBeads  []string
	Events       []activity.ActivityEvent
	HasErrors    bool
}

// ParsePaneContent extracts signals from tmux pane output.
func ParsePaneContent(content, sessionName string) ParsedContent {
	result := ParsedContent{
		Status: activity.StatusUnknown,
	}

	lines := strings.Split(content, "\n")
	// Trim trailing empty lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return result
	}

	// Last meaningful line
	result.LastOutput = strings.TrimSpace(lines[len(lines)-1])
	if len(result.LastOutput) > 200 {
		result.LastOutput = result.LastOutput[:200]
	}

	// Check for errors
	for _, line := range lines {
		for _, pat := range errorPatterns {
			if pat.MatchString(line) {
				result.HasErrors = true
				break
			}
		}
		if result.HasErrors {
			break
		}
	}

	// Check status from the last few lines
	tail := lines
	if len(tail) > 10 {
		tail = tail[len(tail)-10:]
	}

	// Check for active indicators
	for _, line := range tail {
		for _, indicator := range activeIndicators {
			if strings.Contains(line, indicator) {
				result.Status = activity.StatusActive
				break
			}
		}
		if result.Status == activity.StatusActive {
			break
		}
	}

	// If not active, check for prompt (idle)
	if result.Status == activity.StatusUnknown {
		lastLine := lines[len(lines)-1]
		for _, pat := range promptPatterns {
			if pat.MatchString(lastLine) {
				result.Status = activity.StatusIdle
				break
			}
		}
	}

	// Extract files touched
	seen := map[string]bool{}
	for _, line := range lines {
		matches := fileEditPattern.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if !seen[m[1]] {
				seen[m[1]] = true
				result.FilesTouched = append(result.FilesTouched, m[1])
			}
		}
	}

	// Extract active beads
	beadSeen := map[string]bool{}
	for _, line := range lines {
		matches := beadPattern.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if !beadSeen[m[1]] {
				beadSeen[m[1]] = true
				result.ActiveBeads = append(result.ActiveBeads, m[1])
			}
		}
	}

	return result
}

// DetectEventType returns the type of activity seen in a pane content change.
func DetectEventType(content string) string {
	if gitCommitPattern.MatchString(content) {
		return "git_commit"
	}
	if gitPushPattern.MatchString(content) {
		return "git_push"
	}
	if testRunPattern.MatchString(content) {
		return "test_run"
	}
	if fileEditPattern.MatchString(content) {
		return "file_edit"
	}
	return "command_run"
}
