// Package parent provides a watchdog that terminates the process when its
// parent (e.g. the Claude Code session that spawned this MCP server) dies.
//
// MCP servers communicate over stdin/stdout, so the canonical "parent gone"
// signal is EOF on stdin — and the upstream mcp-go library handles that.
// In practice we observe that some terminations (crashes, kill -9, terminal
// closes) leave the stdin pipe open longer than the parent process, which
// causes intermux-mcp instances to accumulate as zombies.
//
// The watchdog backstops EOF detection by polling getppid(). On Unix, a child
// whose parent has died is re-parented to PID 1 (init/launchd). When that's
// observed, the supplied onParentDeath callback fires and the watchdog exits.
package parent

import (
	"context"
	"os"
	"time"
)

// getppid is overridable in tests.
var getppid = os.Getppid

// Watch invokes onParentDeath when the parent process dies. It blocks until
// either ctx is cancelled or parent death is detected, so callers should run
// it in a goroutine.
//
// The interval controls how often getppid() is polled. 30s is a reasonable
// default — getppid is a trivial syscall and 30s is the maximum zombie window.
//
// onParentDeath runs at most once. The caller decides what shutdown means
// (cancel a context, close stdin, os.Exit, etc.).
func Watch(ctx context.Context, interval time.Duration, onParentDeath func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// On Unix, a child whose parent has died is re-parented to PID 1
			// (init/launchd). We only treat ppid == 1 as definitive — other
			// ppid changes can occur in benign re-exec scenarios.
			if getppid() == 1 {
				onParentDeath()
				return
			}
		}
	}
}
