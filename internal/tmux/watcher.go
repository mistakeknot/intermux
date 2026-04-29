package tmux

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mistakeknot/intermux/internal/activity"
	"github.com/mistakeknot/intermux/internal/idle"
)

// WatcherConfig configures the tmux watcher.
type WatcherConfig struct {
	Interval     time.Duration // scan interval (default 10s)
	SessionMatch []string      // session name substrings to match (default: "claude", "codex")
	SocketPath   string        // explicit tmux socket path (e.g. "/tmp/tmux-0/default"); empty = tmux default
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() WatcherConfig {
	return WatcherConfig{
		Interval:     10 * time.Second,
		SessionMatch: []string{"claude", "codex"},
	}
}

// IdleInterval is the scan interval used when the server is idle (no MCP traffic).
// Much longer than the active interval to avoid burning CPU on orphaned processes.
const IdleInterval = 5 * time.Minute

// Watcher continuously scans tmux sessions for agent activity.
type Watcher struct {
	config       WatcherConfig
	store        *activity.Store
	idleTracker  *idle.Tracker
	lastContent  map[string]string    // session -> last pane content hash
	lastChangeAt map[string]time.Time // session -> last content change time
}

// NewWatcher creates a new tmux watcher.
func NewWatcher(config WatcherConfig, store *activity.Store) *Watcher {
	return &Watcher{
		config:       config,
		store:        store,
		lastContent:  make(map[string]string),
		lastChangeAt: make(map[string]time.Time),
	}
}

// SetIdleTracker attaches an idle tracker for adaptive tick rates.
func (w *Watcher) SetIdleTracker(t *idle.Tracker) {
	w.idleTracker = t
}

// Run starts the watcher loop. Blocks until context is cancelled.
// When an idle tracker is attached, the watcher backs off to IdleInterval
// when no MCP traffic has been seen, and resumes the normal interval
// immediately when a request arrives.
func (w *Watcher) Run(ctx context.Context) {
	log.Printf("intermux: tmux watcher started (interval=%s, idle_interval=%s, match=%v)",
		w.config.Interval, IdleInterval, w.config.SessionMatch)

	activeTicker := time.NewTicker(w.config.Interval)
	idleTicker := time.NewTicker(IdleInterval)
	defer activeTicker.Stop()
	defer idleTicker.Stop()

	// Initial scan
	w.scan()

	for {
		if w.idleTracker != nil && w.idleTracker.IsIdle() {
			// Idle mode: long interval, but wake immediately on MCP traffic
			select {
			case <-ctx.Done():
				log.Printf("intermux: tmux watcher stopped")
				return
			case <-idleTicker.C:
				w.scan()
			case <-w.idleTracker.WakeCh():
				log.Printf("intermux: tmux watcher woke from idle")
				w.scan()
				// Reset active ticker so the next tick is a full interval away
				activeTicker.Reset(w.config.Interval)
			}
		} else {
			// Active mode: normal interval
			select {
			case <-ctx.Done():
				log.Printf("intermux: tmux watcher stopped")
				return
			case <-activeTicker.C:
				w.scan()
			}
		}
	}
}

func (w *Watcher) scan() {
	sessions, err := listSessions()
	if err != nil {
		// tmux not running is normal, but log for diagnosability
		log.Printf("intermux: listSessions error: %v", err)
		return
	}

	// Track which sessions are still alive
	alive := make(map[string]bool)

	for _, sess := range sessions {
		if !w.matchesFilter(sess.Name) {
			continue
		}
		alive[sess.Name] = true
		w.scanSession(sess)
	}

	// Detect ended sessions
	for session := range w.lastContent {
		if !alive[session] {
			w.store.PushEvent(activity.ActivityEvent{
				TmuxSession: session,
				Timestamp:   time.Now(),
				EventType:   "session_end",
				Summary:     fmt.Sprintf("tmux session %q ended", session),
			})
			w.store.Remove(session)
			delete(w.lastContent, session)
			delete(w.lastChangeAt, session)
		}
	}
}

func (w *Watcher) matchesFilter(name string) bool {
	// Primary: use structural session name parsing
	parsed := ParseSessionName(name)
	if parsed.IsAgent {
		return true
	}
	// Fallback: substring match for non-standard naming
	lower := strings.ToLower(name)
	for _, match := range w.config.SessionMatch {
		if strings.Contains(lower, strings.ToLower(match)) {
			return true
		}
	}
	return false
}

func (w *Watcher) scanSession(sess sessionInfo) {
	content, err := capturePaneContent(sess.Name)
	if err != nil {
		return
	}

	parsed := ParsePaneContent(content, sess.Name)

	// Detect content change
	prevContent := w.lastContent[sess.Name]
	contentChanged := content != prevContent
	if contentChanged {
		w.lastContent[sess.Name] = content
		w.lastChangeAt[sess.Name] = time.Now()

		// Push activity event
		eventType := DetectEventType(content)
		w.store.PushEvent(activity.ActivityEvent{
			TmuxSession: sess.Name,
			Timestamp:   time.Now(),
			EventType:   eventType,
			Summary:     parsed.LastOutput,
		})
	}

	// Determine status with staleness check
	status := parsed.Status
	if status == activity.StatusActive {
		// Check for stuck: active status but content hasn't changed
		if lastChange, ok := w.lastChangeAt[sess.Name]; ok {
			if time.Since(lastChange) > 5*time.Minute {
				status = activity.StatusStuck
			}
		}
	}

	// Check if process is still alive
	pid := sess.PanePID
	if pid > 0 && !processAlive(pid) {
		status = activity.StatusCrashed
	}

	// Get CWD from process
	cwd := getCWD(pid)
	branch := getGitBranch(cwd)
	projectDir := resolveProjectDir(cwd)

	// Parse session name for structured metadata
	parsedName := ParseSessionName(sess.Name)

	activeBeadID, activeBeadConfidence := observedBeadPresence(parsed.ActiveBeads)

	// Build activity record
	act := activity.AgentActivity{
		TmuxSession:          sess.Name,
		Terminal:             parsedName.Terminal,
		Project:              parsedName.Project,
		AgentType:            parsedName.AgentType,
		AgentNumber:          parsedName.AgentNumber,
		ProjectDir:           projectDir,
		PID:                  pid,
		CWD:                  cwd,
		GitBranch:            branch,
		Status:               status,
		LastOutput:           parsed.LastOutput,
		ActiveBeadID:         activeBeadID,
		ActiveBeadConfidence: activeBeadConfidence,
		ActiveBeads:          parsed.ActiveBeads,
		FilesTouched:         parsed.FilesTouched,
		LastSeen:             time.Now(),
	}

	// Preserve existing agent ID correlation and explicit reported metadata.
	existing := w.store.Get(sess.Name)
	if existing != nil {
		if existing.AgentID != "" {
			act.AgentID = existing.AgentID
		}
		if len(existing.Metadata) > 0 {
			act.Metadata = cloneMetadata(existing.Metadata)
			if reported := strings.TrimSpace(existing.Metadata["active_bead_id"]); reported != "" {
				act.ActiveBeadID = reported
				act.ActiveBeadConfidence = strings.TrimSpace(existing.Metadata["active_bead_confidence"])
				if act.ActiveBeadConfidence == "" {
					act.ActiveBeadConfidence = "reported"
				}
			}
		}
	}

	w.store.Update(sess.Name, act)
}

func cloneMetadata(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func observedBeadPresence(activeBeads []string) (string, string) {
	if len(activeBeads) == 1 {
		return activeBeads[0], "observed"
	}
	return "", "unknown"
}

// --- tmux socket configuration ---

// globalSocketPath holds the tmux socket path for all tmux commands.
// Set via SetSocketPath. Empty means use tmux's default.
var globalSocketPath string

// SetSocketPath configures the tmux socket path used by all commands.
// Pass empty string to use tmux's default socket.
func SetSocketPath(path string) {
	globalSocketPath = path
}

// tmuxArgs prepends -S <socketPath> if configured, then appends the given args.
func tmuxArgs(args ...string) []string {
	if globalSocketPath != "" {
		return append([]string{"-S", globalSocketPath}, args...)
	}
	return args
}

// --- tmux command helpers ---

type sessionInfo struct {
	Name     string
	Width    int
	Height   int
	Created  string
	Attached bool
	Windows  int
	PanePID  int
}

func listSessions() ([]sessionInfo, error) {
	cmd := exec.Command("tmux", tmuxArgs("list-sessions", "-F",
		"#{session_name}\t#{session_width}\t#{session_height}\t#{session_created_string}\t#{session_attached}\t#{session_windows}")...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var sessions []sessionInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 6 {
			continue
		}
		width, _ := strconv.Atoi(parts[1])
		height, _ := strconv.Atoi(parts[2])
		windows, _ := strconv.Atoi(parts[5])

		// Get the pane PID for the active pane
		panePID := getPanePID(parts[0])

		sessions = append(sessions, sessionInfo{
			Name:     parts[0],
			Width:    width,
			Height:   height,
			Created:  parts[3],
			Attached: parts[4] == "1",
			Windows:  windows,
			PanePID:  panePID,
		})
	}
	return sessions, nil
}

func getPanePID(session string) int {
	cmd := exec.Command("tmux", tmuxArgs("list-panes", "-t", session, "-F", "#{pane_pid}")...)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return 0
	}
	pid, _ := strconv.Atoi(lines[0])
	return pid
}

func capturePaneContent(session string) (string, error) {
	cmd := exec.Command("tmux", tmuxArgs("capture-pane", "-t", session, "-p", "-S", "-100")...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// CapturePaneContentFull captures up to 2000 lines for detailed inspection.
func CapturePaneContentFull(session string) (string, error) {
	cmd := exec.Command("tmux", tmuxArgs("capture-pane", "-t", session, "-p", "-S", "-2000")...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ListRawSessions returns session info for the session_info MCP tool.
func ListRawSessions() ([]activity.SessionInfo, error) {
	cmd := exec.Command("tmux", tmuxArgs("list-sessions", "-F",
		"#{session_name}\t#{session_width}\t#{session_height}\t#{session_created_string}\t#{session_attached}\t#{session_windows}")...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var result []activity.SessionInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 6 {
			continue
		}
		width, _ := strconv.Atoi(parts[1])
		height, _ := strconv.Atoi(parts[2])
		windows, _ := strconv.Atoi(parts[5])
		panePID := getPanePID(parts[0])
		parsed := ParseSessionName(parts[0])

		result = append(result, activity.SessionInfo{
			Name:        parts[0],
			Terminal:    parsed.Terminal,
			Project:     parsed.Project,
			AgentType:   parsed.AgentType,
			AgentNumber: parsed.AgentNumber,
			IsAgent:     parsed.IsAgent,
			Width:       width,
			Height:      height,
			Created:     parts[3],
			Attached:    parts[4] == "1",
			Windows:     windows,
			PanePID:     panePID,
		})
	}
	return result, nil
}

// --- process inspection helpers ---

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// Check /proc/<pid>/status
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

func getCWD(pid int) string {
	if pid <= 0 {
		return ""
	}
	link, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return ""
	}
	return link
}

// resolveProjectDir walks up from dir to find the git root directory.
// This gives the actual project path (e.g., "/root/projects/Interverse/os/clavain")
// regardless of what subdirectory the agent's process is sitting in.
func resolveProjectDir(dir string) string {
	if dir == "" {
		return ""
	}
	for d := dir; d != "/" && d != ""; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
	}
	return dir // fallback: return CWD itself
}

func getGitBranch(dir string) string {
	if dir == "" {
		return ""
	}
	// Walk up to find .git
	for d := dir; d != "/" && d != ""; d = filepath.Dir(d) {
		headFile := filepath.Join(d, ".git", "HEAD")
		data, err := os.ReadFile(headFile)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if strings.HasPrefix(content, "ref: refs/heads/") {
			return strings.TrimPrefix(content, "ref: refs/heads/")
		}
		// Detached HEAD
		if len(content) >= 8 {
			return content[:8]
		}
		return content
	}
	return ""
}
