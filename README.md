# intermux

Agent activity visibility for Claude Code via tmux monitoring.

## What This Does

When you have multiple agents running in tmux panes, it's hard to know what each one is doing without switching between them. intermux monitors tmux sessions, detects agent status (active, idle, stuck, crashed), and pushes enriched metadata to intermute so other tools in the ecosystem can see what's happening.

The MCP server provides 7 tools for cross-agent observability: list agents, peek at output, search across sessions, check health, view activity feeds. The monitoring runs on a 10-second tmux scan interval with 30-second metadata pushes to intermute.

## Installation

```bash
/plugin install intermux
```

## MCP Tools

- **list_agents** — Show all detected agents and their status
- **peek_agent** — See recent output from a specific agent
- **search_output** — Search across all agent output
- **agent_health** — Health check with stuck/crash detection
- **activity_feed** — Chronological activity across all agents
- **session_info** — Tmux session metadata
- **who_is_editing** — Which agent is editing which files

## Architecture

```
cmd/intermux-mcp/    Go MCP server (mark3labs/mcp-go)
bin/launch-mcp.sh    Server launcher
```

In-memory activity store with a ring buffer — no persistence, no SQLite. The data is ephemeral and gets rebuilt on restart. Background goroutines handle tmux watching, health monitoring, and intermute metadata push.

Uses `TMUX_SOCKET` env var for custom socket paths.
