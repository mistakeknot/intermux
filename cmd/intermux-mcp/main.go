package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/mistakeknot/intermux/internal/activity"
	"github.com/mistakeknot/intermux/internal/health"
	"github.com/mistakeknot/intermux/internal/idle"
	"github.com/mistakeknot/intermux/internal/parent"
	"github.com/mistakeknot/intermux/internal/push"
	"github.com/mistakeknot/intermux/internal/tmux"
	"github.com/mistakeknot/intermux/internal/tools"
	"github.com/mistakeknot/intermux/internal/version"
)

func main() {
	// Resolve the running artifact's version from the manifest next to the
	// binary — the truth about what is running, not what was compiled in.
	info := version.Resolve()

	// Activity store — shared by watcher, tools, health monitor, and pusher
	store := activity.NewStore(200)

	// Load agent correlation mappings from the per-user mapping directory
	if err := ensureMappingDir(mappingDir()); err != nil {
		log.Printf("intermux: mapping dir unavailable, agent correlation disabled: %v", err)
	}
	loadMappings(store)

	// Background context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Idle tracker — background goroutines back off when no MCP traffic for 60s.
	// This prevents orphaned intermux-mcp processes from burning CPU indefinitely.
	idleTracker := idle.NewTracker(60 * time.Second)

	// Configure tmux socket path (for cross-user access, e.g. claude-user → root's tmux)
	if sp := os.Getenv("TMUX_SOCKET"); sp != "" {
		tmux.SetSocketPath(sp)
		log.Printf("intermux: using tmux socket %s", sp)
	}

	// Start tmux watcher goroutine
	watcherConfig := tmux.DefaultConfig()
	watcher := tmux.NewWatcher(watcherConfig, store)
	watcher.SetIdleTracker(idleTracker)
	go watcher.Run(ctx)

	// Start health monitor goroutine
	monitorConfig := health.DefaultMonitorConfig()
	monitor := health.NewMonitor(monitorConfig, store)
	monitor.SetIdleTracker(idleTracker)
	go monitor.Run(ctx)

	// Start metadata pusher goroutine (pushes to intermute)
	intermuteURL := os.Getenv("INTERMUTE_URL")
	pusher := push.NewPusher(store, intermuteURL, 30*time.Second)
	pusher.SetIdleTracker(idleTracker)
	go pusher.Run(ctx)

	// Start mapping file watcher (checks for new correlation files)
	go watchMappings(ctx, store, idleTracker)

	// Parent-process watchdog — exits when our parent dies.
	// Backstops stdin-EOF detection in mcp-go: if the parent (Claude Code)
	// crashes or is killed without closing the stdin pipe, this catches it.
	// Closing stdin makes ServeStdio's read loop hit EOF and return cleanly.
	go parent.Watch(ctx, 30*time.Second, func() {
		log.Printf("intermux: parent process died, shutting down")
		cancel()
		_ = os.Stdin.Close()
	})

	// MCP server
	s := server.NewMCPServer(
		"intermux",
		info.Version,
		server.WithToolCapabilities(true),
	)

	tools.RegisterAll(s, store, monitor, idleTracker, info)

	// Handle graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		<-sigCh
		log.Printf("intermux: shutting down")
		cancel()
	}()

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "intermux-mcp: %v\n", err)
		os.Exit(1)
	}
}

// mappingDir is where hooks/session-start.sh writes correlation files.
// It is a per-user directory, never the shared /tmp: a mapping file tells
// intermux which agent a tmux session belongs to, so a world-writable
// location would let any local user plant one and relabel a session.
// INTERMUX_MAPPING_DIR overrides; otherwise $XDG_STATE_HOME/intermux/mappings,
// falling back to ~/.local/state/intermux/mappings. The hook computes the
// same path with the same precedence — keep the two in step.
func mappingDir() string {
	if dir := os.Getenv("INTERMUX_MAPPING_DIR"); dir != "" {
		return dir
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "intermux", "mappings")
}

// ensureMappingDir creates the mapping directory owner-only (0700), tightening
// an existing directory's mode if it is wider. The server does this at start
// so the hook's mkdir is a fallback rather than the only guard.
func ensureMappingDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("mapping dir: no home directory and INTERMUX_MAPPING_DIR unset")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

// loadMappings reads intermux-mapping-*.json files from mappingDir to
// correlate tmux sessions with intermute agent IDs.
func loadMappings(store *activity.Store) {
	loadMappingsFrom(mappingDir(), store)
}

// mappingDirTrusted reports whether dir is a directory only its owner can
// reach. Anything group- or world-accessible is refused outright: the
// loader would rather correlate nothing than trust a file a stranger could
// have written.
func mappingDirTrusted(dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o077 == 0
}

// loadMappingsFrom is the directory-injectable core of loadMappings,
// split out so tests can run hermetically against t.TempDir().
func loadMappingsFrom(dir string, store *activity.Store) {
	if !mappingDirTrusted(dir) {
		return
	}
	files, err := filepath.Glob(filepath.Join(dir, "intermux-mapping-*.json"))
	if err != nil {
		return
	}
	for _, f := range files {
		// Regular files only: a symlink planted here would let a mapping
		// point at content outside the trusted directory.
		if info, err := os.Lstat(f); err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var mapping struct {
			SessionID            string `json:"session_id"`
			TmuxSession          string `json:"tmux_session"`
			AgentID              string `json:"agent_id"`
			ActiveBeadID         string `json:"active_bead_id"`
			ActiveBeadConfidence string `json:"active_bead_confidence"`
		}
		if err := json.Unmarshal(data, &mapping); err != nil {
			continue
		}
		if mapping.TmuxSession != "" {
			if mapping.AgentID != "" {
				store.SetAgentCorrelation(mapping.TmuxSession, mapping.AgentID)
			}
			metadata := map[string]string{}
			if mapping.ActiveBeadID != "" {
				metadata["active_bead_id"] = mapping.ActiveBeadID
				if mapping.ActiveBeadConfidence != "" {
					metadata["active_bead_confidence"] = mapping.ActiveBeadConfidence
				} else {
					metadata["active_bead_confidence"] = "reported"
				}
			}
			store.SetAgentMetadata(mapping.TmuxSession, metadata)
		}
	}
}

// watchMappings periodically checks for new mapping files.
// When idle, backs off to 5-minute intervals.
func watchMappings(ctx context.Context, store *activity.Store, tracker *idle.Tracker) {
	const activeInterval = 15 * time.Second
	const idleInterval = 5 * time.Minute

	activeTicker := time.NewTicker(activeInterval)
	idleTicker := time.NewTicker(idleInterval)
	defer activeTicker.Stop()
	defer idleTicker.Stop()

	for {
		if tracker != nil && tracker.IsIdle() {
			select {
			case <-ctx.Done():
				return
			case <-idleTicker.C:
				loadMappings(store)
			case <-tracker.WakeCh():
				loadMappings(store)
				activeTicker.Reset(activeInterval)
			}
		} else {
			select {
			case <-ctx.Done():
				return
			case <-activeTicker.C:
				loadMappings(store)
			}
		}
	}
}
