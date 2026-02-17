# Intermux

> See `AGENTS.md` for full development guide.

## Overview

Persistent Go MCP server for agent activity visibility. Monitors tmux sessions, detects agent status (active/idle/stuck/crashed), and enriches intermute with live metadata.

## Quick Commands

```bash
go build -o bin/intermux-mcp ./cmd/intermux-mcp/   # Build binary
go build ./...                                       # Verify compilation
bash -n hooks/*.sh                                   # Validate hook syntax
```

## Design Decisions (Do Not Re-Ask)

- Go MCP server (mark3labs/mcp-go), bash for hooks
- In-memory activity store with ring buffer (no SQLite)
- Background goroutines for tmux watching, health monitoring, and intermute push
- Agent correlation via /tmp/intermux-mapping-*.json files
- 10-second tmux scan interval, 30-second metadata push interval
