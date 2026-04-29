package tmux

import (
	"regexp"
	"strings"

	"github.com/mistakeknot/intermux/internal/activity"
)

const beadIDExpr = `[A-Za-z][A-Za-z0-9_-]*-[A-Za-z0-9]{3,5}(?:\.[0-9]+)*`

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
	// Bead IDs follow the format: {project}-{3-5 char alphanumeric}[.N[.N...]].
	// Examples: sylveste-kgfi, MediumSetting-ni9, dtla-vjp.5, Sylveste-rsj.1.4.
	beadPattern           = regexp.MustCompile(`\b(` + beadIDExpr + `)\b`)
	bdCommandIDPattern    = regexp.MustCompile(`\bbd\s+(?:show|close|update|note|reopen|claim|start|done)\s+(` + beadIDExpr + `)\b`)
	bdDepCommandPattern   = regexp.MustCompile(`\bbd\s+dep\b`)
	beadReferencePattern  = regexp.MustCompile(`(?i)\bbeads?(?:\s+(?:id|ids))?\s*[:=#]\s*(` + beadIDExpr + `)\b`)
	issueReferencePattern = regexp.MustCompile(`(?i)\bissues?(?:\s+(?:id|ids))?\s*[:=#]\s*(` + beadIDExpr + `)\b`)
	beadStatusLinePattern = regexp.MustCompile(`^\s*[○◐●✓↳↑←]\s+`)

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

	// Extract active beads from explicit Beads context. Avoid treating generic
	// hyphenated shell terms (for example git-push) as task IDs.
	beadSeen := map[string]bool{}
	for _, line := range lines {
		for _, id := range extractBeadIDs(line) {
			if !beadSeen[id] {
				beadSeen[id] = true
				result.ActiveBeads = append(result.ActiveBeads, id)
			}
		}
	}

	return result
}

func extractBeadIDs(line string) []string {
	var ids []string
	for _, m := range bdCommandIDPattern.FindAllStringSubmatch(line, -1) {
		ids = append(ids, m[1])
	}
	if bdDepCommandPattern.MatchString(line) {
		ids = append(ids, beadMatches(line)...)
	}
	if beadStatusLineHasBeadShape(line) {
		ids = append(ids, beadMatches(line)...)
	}

	for _, m := range beadReferencePattern.FindAllStringSubmatch(line, -1) {
		ids = append(ids, m[1])
	}
	for _, m := range issueReferencePattern.FindAllStringSubmatch(line, -1) {
		ids = append(ids, m[1])
	}
	return uniqueStrings(ids)
}

func beadStatusLineHasBeadShape(line string) bool {
	if !beadStatusLinePattern.MatchString(line) {
		return false
	}
	lower := strings.ToLower(line)
	return strings.Contains(line, " · ") || strings.Contains(line, " — ") || strings.Contains(lower, "issue:")
}

func beadMatches(line string) []string {
	var ids []string
	for _, m := range beadPattern.FindAllStringSubmatch(line, -1) {
		ids = append(ids, m[1])
	}
	return uniqueStrings(ids)
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
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
