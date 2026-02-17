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

	// Configure tmux socket path (for cross-user access, e.g. claude-user → root's tmux)
	if sp := os.Getenv("TMUX_SOCKET"); sp != "" {
		tmux.SetSocketPath(sp)
		log.Printf("intermux: using tmux socket %s", sp)
	}

	// Start tmux watcher goroutine
	watcherConfig := tmux.DefaultConfig()
	watcher := tmux.NewWatcher(watcherConfig, store)
	go watcher.Run(ctx)

	// Start health monitor goroutine
	monitorConfig := health.DefaultMonitorConfig()
	monitor := health.NewMonitor(monitorConfig, store)
	go monitor.Run(ctx)

	// Start metadata pusher goroutine (pushes to intermute)
	intermuteURL := os.Getenv("INTERMUTE_URL")
	pusher := push.NewPusher(store, intermuteURL, 30*time.Second)
	go pusher.Run(ctx)

	// Start mapping file watcher (checks for new correlation files)
	go watchMappings(ctx, store)

	// MCP server
	s := server.NewMCPServer(
		"intermux",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	tools.RegisterAll(s, store, monitor)

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
func watchMappings(ctx context.Context, store *activity.Store) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			loadMappings(store)
		}
	}
}
