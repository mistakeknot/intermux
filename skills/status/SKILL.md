# /intermux:status — Agent Activity Dashboard

<skill-description>Show a live dashboard of all agent tmux sessions with status, activity, and health warnings.</skill-description>

<command-name>status</command-name>

## Instructions

When the user invokes `/intermux:status`, show a comprehensive agent activity dashboard.

### Steps

1. Call the `list_agents` MCP tool from the intermux server to get all detected agent sessions
2. Call the `agent_health` MCP tool to get health reports with warnings
3. Format the results as a clear table:

```
Agent Activity Dashboard
========================

| Session | Status | CWD | Branch | Last Activity | Beads |
|---------|--------|-----|--------|---------------|-------|
| ...     | ...    | ... | ...    | ...           | ...   |

Health Warnings:
- [session]: [warning message]
```

### Status Icons
- `active` — agent is processing
- `idle` — agent is waiting for input
- `stuck` — no output change for >5 minutes
- `crashed` — agent process not found
- `unknown` — can't determine

### Additional Context
- If multiple agents are editing files in the same directory, highlight this as a potential conflict
- If any agent is stuck or crashed, call attention to it prominently
- Show the total number of active vs total agents
