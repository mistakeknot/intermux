package tmux

import (
	"os"
	"path/filepath"
	"testing"
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
