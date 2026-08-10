package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/mistakeknot/intermux/internal/activity"
)

func callListAgents(t *testing.T, ctx context.Context, store *activity.Store, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
	res, err := listAgents(store).Handler(ctx, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	return res
}

func resultAgents(t *testing.T, res *mcp.CallToolResult) []activity.AgentActivity {
	t.Helper()
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}
	text, ok := mcp.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("expected text content, got %T", res.Content[0])
	}
	var agents []activity.AgentActivity
	if err := json.Unmarshal([]byte(text.Text), &agents); err != nil {
		t.Fatalf("unmarshal agents: %v", err)
	}
	return agents
}

// Before the initial scan completes, list_agents must refuse rather than
// serve whatever partial fleet happens to be in the store. On the old
// behavior this test fails: the handler returned the partial immediately.
func TestListAgentsRefusesMidScanPartial(t *testing.T) {
	store := activity.NewStore(10)
	// A partial: one session already scanned in, others still pending.
	store.Update("iterm[jeddnet@codex - 019f8", activity.AgentActivity{
		TmuxSession: "iterm[jeddnet@codex - 019f8", Project: "jeddnet",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	res := callListAgents(t, ctx, store, nil)
	if !res.IsError {
		t.Fatal("expected error result while initial scan incomplete, got data (mid-scan partial served)")
	}
}

// A call already blocked in the gate must observe every store update the
// initial scan made before MarkScanComplete — the full fleet, not a partial.
func TestListAgentsUnblocksWithCompleteFleet(t *testing.T) {
	store := activity.NewStore(10)
	store.Update("iterm[jeddnet@codex - 019f8", activity.AgentActivity{
		TmuxSession: "iterm[jeddnet@codex - 019f8", Project: "jeddnet",
	})

	go func() {
		time.Sleep(30 * time.Millisecond)
		// The "rest" of the initial scan lands, then completion is marked.
		store.Update("rio[clavain - aa2b7", activity.AgentActivity{
			TmuxSession: "rio[clavain - aa2b7", Project: "clavain",
		})
		store.MarkScanComplete()
	}()

	res := callListAgents(t, context.Background(), store, nil)
	agents := resultAgents(t, res)
	if len(agents) != 2 {
		t.Fatalf("got %d agents, want the complete fleet of 2: %+v", len(agents), agents)
	}
}

// Every fleet-reading tool must refuse while the initial scan is incomplete
// and serve normally once it is marked. On ungated behavior this fails:
// the handlers returned data immediately.
func TestFleetReadersGateOnScanComplete(t *testing.T) {
	cases := []struct {
		name string
		tool func(*activity.Store) server.ServerTool
		args map[string]any
	}{
		{"activity_feed", activityFeed, nil},
		{"search_output", searchOutput, map[string]any{"pattern": "error"}},
		{"who_is_editing", whoIsEditing, map[string]any{"pattern": "main.go"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := activity.NewStore(10)
			store.Update("iterm[demo - 019f8", activity.AgentActivity{TmuxSession: "iterm[demo - 019f8"})
			req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: tc.args}}

			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			res, err := tc.tool(store).Handler(ctx, req)
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if !res.IsError {
				t.Fatal("expected error while initial scan incomplete, got data (mid-scan partial served)")
			}

			store.MarkScanComplete()
			res, err = tc.tool(store).Handler(context.Background(), req)
			if err != nil {
				t.Fatalf("handler error after scan complete: %v", err)
			}
			if res.IsError {
				t.Fatalf("expected data after scan complete, got error: %+v", res.Content)
			}
		})
	}
}

// Argument validation must fail fast even while the gate would block:
// a malformed request's error is about the request, not scan state.
func TestArgValidationRunsBeforeGate(t *testing.T) {
	store := activity.NewStore(10) // scan deliberately never marked complete
	for name, st := range map[string]server.ServerTool{
		"search_output":  searchOutput(store),
		"who_is_editing": whoIsEditing(store),
	} {
		start := time.Now()
		res, err := st.Handler(context.Background(), mcp.CallToolRequest{})
		if err != nil {
			t.Fatalf("%s: handler error: %v", name, err)
		}
		if !res.IsError {
			t.Fatalf("%s: expected argument error", name)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("%s: argument error took %v — blocked on the scan gate", name, elapsed)
		}
	}
}

// Once the scan is complete the gate must be free — no added latency, and
// filters keep working.
func TestListAgentsAfterScanCompleteFilters(t *testing.T) {
	store := activity.NewStore(10)
	store.Update("iterm[jeddnet@codex - 019f8", activity.AgentActivity{
		TmuxSession: "iterm[jeddnet@codex - 019f8", Project: "jeddnet", AgentType: "codex",
	})
	store.Update("rio[clavain - aa2b7", activity.AgentActivity{
		TmuxSession: "rio[clavain - aa2b7", Project: "clavain", AgentType: "claude",
	})
	store.MarkScanComplete()

	res := callListAgents(t, context.Background(), store, map[string]any{"project": "jeddnet"})
	agents := resultAgents(t, res)
	if len(agents) != 1 || agents[0].Project != "jeddnet" {
		t.Fatalf("project filter broken after gate: %+v", agents)
	}
}
