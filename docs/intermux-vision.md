# intermux — Vision and Philosophy

**Version:** 0.1.0
**Last updated:** 2026-02-28

## What intermux Is

intermux is a persistent Go MCP server that makes agent activity observable. It monitors tmux sessions, classifies agent status (active/idle/stuck/crashed), extracts context (working directory, git branch, active beads, files touched), and pushes live metadata to intermute via sideband. The activity store is in-memory with a ring buffer — lightweight by design, not a database pretending to be a monitor.

Session names encode identity: `{terminal}-{project}-{agent}-{number}` gives intermux enough signal to correlate sessions to agents without requiring agents to self-report. Mapping files at `/tmp/intermux-mapping-*.json` bridge the gap when session names alone aren't enough.

## Why This Exists

Multi-agent workflows are opaque by default. When five agents are running across a server, you can't tell which is stuck, which is idle waiting for input, and which is actively doing useful work. Without that signal, humans over-supervise (checking panes manually) or under-supervise (missing hung agents for hours). intermux closes that loop: it instruments the environment agents already use — tmux — and surfaces the state that was always there but invisible.

## Design Principles

1. **Instrument what already exists.** Agents run in tmux. tmux exposes session state. intermux reads that state on a 10-second scan interval rather than requiring agents to emit structured telemetry. Zero cooperation needed from agent code. (Every action produces evidence — intermux makes that evidence visible.)

2. **Status is inferred, not self-reported.** Active/idle/stuck/crashed are derived from pane output patterns, process liveness, and time since last change. Self-reporting creates a coordination problem; observation doesn't.

3. **Small scope, explicit interface.** intermux does one thing: collect and push agent activity metadata. It does not orchestrate, route, or decide. It feeds intermute, which feeds the agents that do. (Composition over capability — the platform composes; plugins observe.)

4. **Cheap to run, cheap to lose.** In-memory ring buffer, no persistence layer, no SQLite. If intermux restarts, the next scan cycle rebuilds current state in seconds. The 30-second intermute push interval means metadata lag is bounded and acceptable.

5. **Measurement before optimization.** Knowing which agents are stuck is the prerequisite for fixing stuck agents automatically. intermux is the measurement layer; remediation belongs upstream.

## Scope

**Does:**
- Monitor tmux sessions and classify agent status every 10 seconds
- Parse session names to extract terminal, project, agent type, and instance number
- Track CWD, git branch, active beads, and recent file touches from pane content
- Correlate sessions to intermute agent IDs via `/tmp/intermux-mapping-*.json`
- Push activity snapshots to intermute every 30 seconds
- Expose MCP tools for querying live agent state

**Does not:**
- Persist history across restarts
- Make routing or scheduling decisions
- Monitor non-tmux processes directly
- Replace intermute — it feeds intermute

## Direction

- Harden stuck detection heuristics: current threshold (>5 min no change while active) needs calibration against real workload patterns
- Surface intermux data in the agent overlay UI (roadmap item iv-9kq3: F5 agent overlay)
- Add bead-awareness: correlate active bead IDs to session state so intermute can infer which bead is blocked
