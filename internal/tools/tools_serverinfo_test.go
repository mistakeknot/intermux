package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mistakeknot/intermux/internal/activity"
	"github.com/mistakeknot/intermux/internal/version"
)

// server_info is the diagnostic surface: it must answer immediately with the
// artifact's true version and honest scan state — never block on the gate.
func TestServerInfoReportsVersionAndScanState(t *testing.T) {
	store := activity.NewStore(10)
	st := serverInfo(store, version.Info{Version: "1.2.3", Binary: "/x/bin/intermux-mcp"}, time.Now())

	call := func() map[string]any {
		t.Helper()
		start := time.Now()
		res, err := st.Handler(context.Background(), mcp.CallToolRequest{})
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %+v", res.Content)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("server_info took %v — it must never block on the scan gate", elapsed)
		}
		text, ok := mcp.AsTextContent(res.Content[0])
		if !ok {
			t.Fatalf("expected text content, got %T", res.Content[0])
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return out
	}

	out := call()
	if out["version"] != "1.2.3" {
		t.Fatalf("version = %v, want 1.2.3", out["version"])
	}
	if out["scan_complete"] != false {
		t.Fatalf("scan_complete = %v before scan, want false", out["scan_complete"])
	}

	store.MarkScanComplete()
	if out := call(); out["scan_complete"] != true {
		t.Fatalf("scan_complete = %v after scan, want true", out["scan_complete"])
	}
}
