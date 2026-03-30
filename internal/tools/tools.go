package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/mistakeknot/intermux/internal/activity"
	"github.com/mistakeknot/intermux/internal/health"
	"github.com/mistakeknot/intermux/internal/tmux"
)

// RegisterAll registers all intermux MCP tools with the server.
func RegisterAll(s *server.MCPServer, store *activity.Store, monitor *health.Monitor) {
	s.AddTools(
		listAgents(store),
		peekAgent(store),
		activityFeed(store),
		searchOutput(store),
		agentHealth(monitor),
		whoIsEditing(store),
		sessionInfo(),
	)
}

func listAgents(store *activity.Store) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_agents",
			mcp.WithDescription("List all detected agent tmux sessions with status, working directory, git branch, and activity summary. Each result includes parsed session name fields (terminal, project, agent_type, agent_number) and project_dir (git root resolved from CWD). The 'project' field comes from the session name convention {terminal}-{project}-{agent}-{N}; project_dir comes from the actual filesystem."),
			mcp.WithString("project",
				mcp.Description("Filter to agents working on this project (case-insensitive substring match on parsed project name OR project_dir path)"),
			),
			mcp.WithString("agent_type",
				mcp.Description("Filter by agent type: 'claude', 'codex', 'dev'"),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			projectFilter, _ := args["project"].(string)
			agentTypeFilter, _ := args["agent_type"].(string)

			agents := store.List()
			if projectFilter != "" || agentTypeFilter != "" {
				var filtered []activity.AgentActivity
				for _, a := range agents {
					if projectFilter != "" &&
						!strings.Contains(strings.ToLower(a.Project), strings.ToLower(projectFilter)) &&
						!strings.Contains(strings.ToLower(a.ProjectDir), strings.ToLower(projectFilter)) {
						continue
					}
					if agentTypeFilter != "" && !strings.EqualFold(a.AgentType, agentTypeFilter) {
						continue
					}
					filtered = append(filtered, a)
				}
				agents = filtered
			}
			if agents == nil {
				agents = []activity.AgentActivity{}
			}
			return jsonResult(agents)
		},
	}
}

func peekAgent(store *activity.Store) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("peek_agent",
			mcp.WithDescription("Detailed view of one agent: last N lines of pane output, process info, and full activity record."),
			mcp.WithString("session",
				mcp.Description("Tmux session name to peek at"),
				mcp.Required(),
			),
			mcp.WithNumber("lines",
				mcp.Description("Number of pane output lines to capture (default 50, max 200)"),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			session, _ := args["session"].(string)
			if session == "" {
				return mcp.NewToolResultError("session is required"), nil
			}
			lines := intOr(args["lines"], 50)
			if lines > 200 {
				lines = 200
			}

			act := store.Get(session)
			if act == nil {
				return mcp.NewToolResultError(fmt.Sprintf("no agent found for session %q", session)), nil
			}

			// Live capture for freshest data
			content, err := tmux.CapturePaneContentFull(session)
			if err != nil {
				content = "(could not capture pane: " + err.Error() + ")"
			}

			// Trim to requested lines
			contentLines := strings.Split(content, "\n")
			if len(contentLines) > lines {
				contentLines = contentLines[len(contentLines)-lines:]
			}

			return jsonResult(map[string]any{
				"activity":     act,
				"pane_content": strings.Join(contentLines, "\n"),
				"captured_at":  time.Now().Format(time.RFC3339),
			})
		},
	}
}

func activityFeed(store *activity.Store) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("activity_feed",
			mcp.WithDescription("Chronological stream of recent events across all agents. Filter by time window, session, or event type."),
			mcp.WithNumber("minutes",
				mcp.Description("Look back this many minutes (default 10)"),
			),
			mcp.WithString("session",
				mcp.Description("Filter events to this tmux session"),
			),
			mcp.WithString("event_type",
				mcp.Description("Filter by event type: file_edit, command_run, git_commit, git_push, test_run, error, session_start, session_end"),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			minutes := intOr(args["minutes"], 10)
			session, _ := args["session"].(string)
			eventType, _ := args["event_type"].(string)

			since := time.Now().Add(-time.Duration(minutes) * time.Minute)
			events := store.Feed(since, session, eventType)
			if events == nil {
				events = []activity.ActivityEvent{}
			}
			return jsonResult(events)
		},
	}
}

func searchOutput(store *activity.Store) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("search_output",
			mcp.WithDescription("Search all agent pane content for a pattern (e.g., 'error', 'test failed'). Searches both the event history and live pane content."),
			mcp.WithString("pattern",
				mcp.Description("Search pattern (case-insensitive substring match)"),
				mcp.Required(),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			pattern, _ := args["pattern"].(string)
			if pattern == "" {
				return mcp.NewToolResultError("pattern is required"), nil
			}

			type searchResult struct {
				TmuxSession string `json:"tmux_session"`
				Line        string `json:"line"`
				LineNumber  int    `json:"line_number"`
			}

			var results []searchResult
			re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(pattern))
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid pattern: %v", err)), nil
			}

			// Search live pane content across all known sessions
			agents := store.List()
			for _, agent := range agents {
				content, err := tmux.CapturePaneContentFull(agent.TmuxSession)
				if err != nil {
					continue
				}
				lines := strings.Split(content, "\n")
				for i, line := range lines {
					if re.MatchString(line) {
						results = append(results, searchResult{
							TmuxSession: agent.TmuxSession,
							Line:        strings.TrimSpace(line),
							LineNumber:  i + 1,
						})
					}
				}
			}

			return jsonResult(map[string]any{
				"pattern": pattern,
				"matches": results,
				"count":   len(results),
			})
		},
	}
}

func agentHealth(monitor *health.Monitor) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("agent_health",
			mcp.WithDescription("Health report for all agents: which are active, idle, stuck, or crashed. Includes warnings."),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			reports := monitor.Report()
			if reports == nil {
				reports = []health.Report{}
			}
			return jsonResult(reports)
		},
	}
}

func whoIsEditing(store *activity.Store) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("who_is_editing",
			mcp.WithDescription("Check if any agent is actively editing files matching a pattern. Useful before starting work on a file."),
			mcp.WithString("pattern",
				mcp.Description("File path pattern to check (substring match)"),
				mcp.Required(),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			pattern, _ := args["pattern"].(string)
			if pattern == "" {
				return mcp.NewToolResultError("pattern is required"), nil
			}

			type editActivity struct {
				TmuxSession string   `json:"tmux_session"`
				AgentID     string   `json:"agent_id,omitempty"`
				Files       []string `json:"matching_files"`
				LastSeen    string   `json:"last_seen"`
			}

			var results []editActivity
			agents := store.List()
			for _, agent := range agents {
				var matching []string
				for _, f := range agent.FilesTouched {
					if strings.Contains(strings.ToLower(f), strings.ToLower(pattern)) {
						matching = append(matching, f)
					}
				}
				if len(matching) > 0 {
					results = append(results, editActivity{
						TmuxSession: agent.TmuxSession,
						AgentID:     agent.AgentID,
						Files:       matching,
						LastSeen:    agent.LastSeen.Format(time.RFC3339),
					})
				}
			}

			return jsonResult(map[string]any{
				"pattern": pattern,
				"editors": results,
				"count":   len(results),
			})
		},
	}
}

func sessionInfo() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("session_info",
			mcp.WithDescription("Raw tmux session info: name, size, created time, attached/detached status, pane PID."),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sessions, err := tmux.ListRawSessions()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("list sessions: %v", err)), nil
			}
			if sessions == nil {
				sessions = []activity.SessionInfo{}
			}
			return jsonResult(sessions)
		},
	}
}

// --- Helpers ---

func jsonResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func intOr(v any, def int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return def
}
