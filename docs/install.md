# Standalone install

intermux is a normal Go MCP server — no Claude Code plugin required.

1. Install the binary:

```bash
go install github.com/mistakeknot/intermux/cmd/intermux-mcp@latest
```

2. Point any MCP client at it. Raw config for any client:

```json
{
  "mcpServers": {
    "intermux": {
      "command": "intermux-mcp",
      "env": {
        "INTERMUTE_URL": "http://127.0.0.1:7338"
      }
    }
  }
}
```

`INTERMUTE_URL` is optional: without intermute, push is disabled and every
tool still works — intermux watches tmux and answers `list_agents`,
`peek_agent`, `search_output`, `agent_health`, `activity_feed`,
`session_info`, `who_is_editing`, and `server_info` on its own. Set it to
push enriched agent metadata (status, CWD, git branch, active bead) to a
running [intermute](https://github.com/mistakeknot/intermute) instance so
other tools on the same machine can see it.

3. Claude Code plugin path (marketplace) is also available — see the
   README § Installation.
