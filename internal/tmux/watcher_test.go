package tmux

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mistakeknot/intermux/internal/activity"
)

func TestProcessAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Error("own pid should be alive")
	}
	if processAlive(0) {
		t.Error("pid 0 should never be alive")
	}
	if processAlive(-1) {
		t.Error("negative pid should never be alive")
	}
	// PID 1 (launchd/systemd) exists on both platforms but is not ours,
	// so this exercises the EPERM-still-means-alive path when unprivileged.
	if !processAlive(1) {
		t.Error("pid 1 should be reported alive")
	}
}

func TestGetCWD(t *testing.T) {
	got := getCWD(os.Getpid())
	if got == "" {
		t.Fatal("expected a cwd for our own pid")
	}
	want, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Resolve symlinks on both sides: on macOS lsof reports /private/tmp
	// paths where Getwd may say /tmp.
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("resolving got %q: %v", got, err)
	}
	wantResolved, err := filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatalf("resolving want %q: %v", want, err)
	}
	if gotResolved != wantResolved {
		t.Errorf("cwd: got %q, want %q", gotResolved, wantResolved)
	}
}

func TestGetCWDInvalidPid(t *testing.T) {
	if got := getCWD(0); got != "" {
		t.Errorf("expected empty cwd for pid 0, got %q", got)
	}
}

// Run must mark scan completion even when tmux is unreachable (listSessions
// errors): zero sessions is a truthful, complete answer. Without the mark,
// every gated reader would block until its timeout on machines with no tmux.
func TestRunMarksScanCompleteEvenWithoutTmux(t *testing.T) {
	// Point tmux at a socket that cannot exist so listSessions fails fast
	// and deterministically on every platform.
	old := globalSocketPath
	SetSocketPath(filepath.Join(t.TempDir(), "no-such-socket"))
	t.Cleanup(func() { SetSocketPath(old) })

	store := activity.NewStore(10)
	w := NewWatcher(DefaultConfig(), store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	if !store.WaitScanComplete(context.Background(), 5*time.Second) {
		t.Fatal("initial scan was never marked complete")
	}
}
