package main

import (
	"os"
	"path/filepath"
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
