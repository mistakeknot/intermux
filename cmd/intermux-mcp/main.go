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
)

func main() {
	// Activity store — shared by watcher, tools, health monitor, and pusher
	store := activity.NewStore(200)

	// Load agent correlation mappings from /tmp
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
		"0.1.5",
		server.WithToolCapabilities(true),
	)

	tools.RegisterAll(s, store, monitor, idleTracker)

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

// loadMappings reads /tmp/intermux-mapping-*.json files to correlate
// tmux sessions with intermute agent IDs.
func loadMappings(store *activity.Store) {
	files, err := filepath.Glob("/tmp/intermux-mapping-*.json")
	if err != nil {
		return
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var mapping struct {
			SessionID   string `json:"session_id"`
			TmuxSession string `json:"tmux_session"`
			AgentID     string `json:"agent_id"`
		}
		if err := json.Unmarshal(data, &mapping); err != nil {
			continue
		}
		if mapping.TmuxSession != "" && mapping.AgentID != "" {
			store.SetAgentCorrelation(mapping.TmuxSession, mapping.AgentID)
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
