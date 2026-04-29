package push

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/mistakeknot/intermux/internal/activity"
)

func TestBuildMetadataPublishesPresenceContract(t *testing.T) {
	lastSeen := time.Date(2026, 4, 29, 3, 45, 0, 0, time.UTC)
	agent := activity.AgentActivity{
		AgentID:      "agent-123",
		TmuxSession:  "ghostty-sylveste-claude",
		AgentType:    "claude",
		ProjectDir:   "/home/mk/projects/Sylveste",
		CWD:          "/home/mk/projects/Sylveste/interverse/intermux",
		GitBranch:    "main",
		Status:       activity.StatusActive,
		LastOutput:   "bd show sylveste-kgfi.1 --long",
		ActiveBeads:  []string{"sylveste-kgfi.1"},
		FilesTouched: []string{"internal/tmux/parser.go", "internal/push/pusher.go"},
		LastSeen:     lastSeen,
	}

	meta := buildMetadata(agent)

	checks := map[string]string{
		"tmux_session":           "ghostty-sylveste-claude",
		"agent_kind":             "claude",
		"repo":                   "/home/mk/projects/Sylveste",
		"cwd":                    "/home/mk/projects/Sylveste/interverse/intermux",
		"git_branch":             "main",
		"status":                 "active",
		"last_seen":              lastSeen.Format(time.RFC3339),
		"active_bead_id":         "sylveste-kgfi.1",
		"active_bead_confidence": "observed",
	}
	for key, want := range checks {
		if got := meta[key]; got != want {
			t.Fatalf("meta[%q] = %q, want %q (all meta: %#v)", key, got, want, meta)
		}
	}

	var files []string
	if err := json.Unmarshal([]byte(meta["files_touched"]), &files); err != nil {
		t.Fatalf("files_touched is not JSON array: %v", err)
	}
	wantFiles := []string{"internal/tmux/parser.go", "internal/push/pusher.go"}
	if !reflect.DeepEqual(files, wantFiles) {
		t.Fatalf("files_touched = %#v, want %#v", files, wantFiles)
	}
}

func TestBuildMetadataPrefersReportedActiveBeadID(t *testing.T) {
	agent := activity.AgentActivity{
		TmuxSession: "ghostty-sylveste-claude",
		AgentType:   "claude",
		Status:      activity.StatusActive,
		ActiveBeads: []string{"sylveste-other", "sylveste-kgfi.1"},
		Metadata: map[string]string{
			"active_bead_id": "sylveste-kgfi.1",
		},
		LastSeen: time.Date(2026, 4, 29, 3, 45, 0, 0, time.UTC),
	}

	meta := buildMetadata(agent)
	if got := meta["active_bead_id"]; got != "sylveste-kgfi.1" {
		t.Fatalf("active_bead_id = %q, want reported sylveste-kgfi.1", got)
	}
	if got := meta["active_bead_confidence"]; got != "reported" {
		t.Fatalf("active_bead_confidence = %q, want reported", got)
	}
}

func TestBuildMetadataDoesNotGuessAmbiguousActiveBeadID(t *testing.T) {
	agent := activity.AgentActivity{
		TmuxSession: "ghostty-sylveste-claude",
		AgentType:   "claude",
		Status:      activity.StatusActive,
		ActiveBeads: []string{"sylveste-a111", "sylveste-b222"},
		LastSeen:    time.Date(2026, 4, 29, 3, 45, 0, 0, time.UTC),
	}

	meta := buildMetadata(agent)
	if got := meta["active_bead_id"]; got != "" {
		t.Fatalf("active_bead_id = %q, want empty string for ambiguous candidates", got)
	}
	if got := meta["thread_id"]; got != "" {
		t.Fatalf("thread_id = %q, want empty string for ambiguous candidates", got)
	}
	if got := meta["active_bead_confidence"]; got != "unknown" {
		t.Fatalf("active_bead_confidence = %q, want unknown", got)
	}
	var candidates []string
	if err := json.Unmarshal([]byte(meta["active_bead_candidates"]), &candidates); err != nil {
		t.Fatalf("active_bead_candidates is not JSON array: %v", err)
	}
	want := []string{"sylveste-a111", "sylveste-b222"}
	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("active_bead_candidates = %#v, want %#v", candidates, want)
	}
}

func TestBuildMetadataClearsMergeOnlyPresenceKeysWhenUnknown(t *testing.T) {
	agent := activity.AgentActivity{
		TmuxSession: "ghostty-sylveste-claude",
		AgentType:   "claude",
		Status:      activity.StatusIdle,
		LastSeen:    time.Date(2026, 4, 29, 3, 45, 0, 0, time.UTC),
	}

	meta := buildMetadata(agent)
	checks := map[string]string{
		"active_bead_id":         "",
		"thread_id":              "",
		"active_bead_confidence": "unknown",
		"active_beads":           "[]",
		"active_bead_candidates": "[]",
		"files_touched":          "[]",
	}
	for key, want := range checks {
		if got := meta[key]; got != want {
			t.Fatalf("meta[%q] = %q, want %q (all meta: %#v)", key, got, want, meta)
		}
	}
}
