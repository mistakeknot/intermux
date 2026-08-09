package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
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
