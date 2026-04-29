package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/intermux/internal/activity"
)

func TestLoadMappingsPreservesReportedActiveBeadMetadata(t *testing.T) {
	store := activity.NewStore(10)
	store.Update("intermux-test-session", activity.AgentActivity{TmuxSession: "intermux-test-session"})

	path := filepath.Join(os.TempDir(), fmt.Sprintf("intermux-mapping-test-%d.json", os.Getpid()))
	t.Cleanup(func() { _ = os.Remove(path) })
	payload := `{"session_id":"test","tmux_session":"intermux-test-session","agent_id":"agent-123","active_bead_id":"sylveste-kgfi.1","active_bead_confidence":"reported"}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write mapping: %v", err)
	}

	loadMappings(store)

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
