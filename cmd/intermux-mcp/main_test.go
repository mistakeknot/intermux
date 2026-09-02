package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/intermux/internal/activity"
)

func TestLoadMappingsPreservesReportedActiveBeadMetadata(t *testing.T) {
	store := activity.NewStore(10)
	store.Update("intermux-test-session", activity.AgentActivity{TmuxSession: "intermux-test-session"})

	// Hermetic mapping dir: the production path is hardcoded /tmp (matching
	// hooks/session-start.sh), which os.TempDir() only equals on Linux —
	// writing there made this test fail on macOS and read real mapping
	// files on dev machines.
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil { // the loader refuses anything wider
		t.Fatal(err)
	}
	payload := `{"session_id":"test","tmux_session":"intermux-test-session","agent_id":"agent-123","active_bead_id":"sylveste-kgfi.1","active_bead_confidence":"reported"}`
	if err := os.WriteFile(filepath.Join(dir, "intermux-mapping-test.json"), []byte(payload), 0o600); err != nil {
		t.Fatalf("write mapping: %v", err)
	}

	loadMappingsFrom(dir, store)

	got := store.Get("intermux-test-session")
	if got == nil {
		t.Fatal("expected mapped session to remain in store")
	}
	if got.AgentID != "agent-123" {
		t.Fatalf("AgentID = %q, want agent-123", got.AgentID)
	}
	if got.Metadata["active_bead_id"] != "sylveste-kgfi.1" {
		t.Fatalf("active_bead_id metadata = %q, want sylveste-kgfi.1", got.Metadata["active_bead_id"])
	}
	if got.Metadata["active_bead_confidence"] != "reported" {
		t.Fatalf("active_bead_confidence = %q, want reported", got.Metadata["active_bead_confidence"])
	}
}

func TestMappingDirIsPerUserAndOverridable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("INTERMUX_MAPPING_DIR", "")

	want := filepath.Join(home, ".local", "state", "intermux", "mappings")
	if got := mappingDir(); got != want {
		t.Fatalf("default mappingDir = %q, want %q", got, want)
	}
	if !strings.HasPrefix(mappingDir(), home+string(os.PathSeparator)) {
		t.Fatalf("default mapping dir must live under the user's home, got %q", mappingDir())
	}

	xdg := filepath.Join(home, "xdg-state")
	t.Setenv("XDG_STATE_HOME", xdg)
	if got, want := mappingDir(), filepath.Join(xdg, "intermux", "mappings"); got != want {
		t.Fatalf("XDG mappingDir = %q, want %q", got, want)
	}

	explicit := filepath.Join(home, "explicit")
	t.Setenv("INTERMUX_MAPPING_DIR", explicit)
	if got := mappingDir(); got != explicit {
		t.Fatalf("INTERMUX_MAPPING_DIR mappingDir = %q, want %q", got, explicit)
	}
}

func TestEnsureMappingDirIsOwnerOnly(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "mappings")
	if err := ensureMappingDir(dir); err != nil {
		t.Fatalf("ensureMappingDir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("mode = %o, want 0700", got)
	}

	// An existing wider directory is tightened, not trusted as-is.
	wide := filepath.Join(t.TempDir(), "wide")
	if err := os.Mkdir(wide, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(wide, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureMappingDir(wide); err != nil {
		t.Fatalf("ensureMappingDir(wide): %v", err)
	}
	info, _ = os.Stat(wide)
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("wide dir mode after ensure = %o, want 0700", got)
	}
}

func TestLoadMappingsRefusesSharedDirectoryAndSymlinks(t *testing.T) {
	payload := `{"session_id":"x","tmux_session":"intermux-shared-session","agent_id":"planted-agent"}`

	// Group/world-accessible directory: refused outright, even with a valid file.
	shared := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "intermux-mapping-x.json"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	store := activity.NewStore(10)
	store.Update("intermux-shared-session", activity.AgentActivity{TmuxSession: "intermux-shared-session"})
	loadMappingsFrom(shared, store)
	if got := store.Get("intermux-shared-session"); got != nil && got.AgentID != "" {
		t.Fatalf("loader trusted a world-readable mapping dir: AgentID=%q", got.AgentID)
	}

	// Owner-only directory, but the mapping file is a symlink out of it: skipped.
	private := filepath.Join(t.TempDir(), "private")
	if err := ensureMappingDir(private); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(private, "intermux-mapping-link.json")); err != nil {
		t.Fatal(err)
	}
	store2 := activity.NewStore(10)
	store2.Update("intermux-shared-session", activity.AgentActivity{TmuxSession: "intermux-shared-session"})
	loadMappingsFrom(private, store2)
	if got := store2.Get("intermux-shared-session"); got != nil && got.AgentID != "" {
		t.Fatalf("loader followed a symlinked mapping file: AgentID=%q", got.AgentID)
	}

	// Same directory, a regular file: loaded.
	if err := os.WriteFile(filepath.Join(private, "intermux-mapping-real.json"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	loadMappingsFrom(private, store2)
	if got := store2.Get("intermux-shared-session"); got == nil || got.AgentID != "planted-agent" {
		t.Fatalf("regular file in owner-only dir should load; got %+v", got)
	}
}
