# Status detection

How intermux decides whether an agent is `active`, `idle`, `stuck`, `crashed`,
or `unknown`. All of this lives in `internal/tmux/parser.go` (pane-content
parsing), `internal/tmux/watcher.go` (the scan loop that calls it), and
`internal/health/monitor.go` (the staleness sweep). This is a heuristic
layer over free-form terminal text, not a protocol — treat the classification
as a best guess, not ground truth.

## The scan

The watcher (`internal/tmux/watcher.go`) captures each tmux pane's visible
content every 10 seconds (`DefaultConfig().Interval`) via
`tmux capture-pane -p -S -100` — up to the last 100 lines. That content is
handed to `ParsePaneContent`, which classifies status from a much smaller
window inside it.

## The 10-line window, and why only its last line counts

`ParsePaneContent` trims trailing blank lines, then looks at a trailing
window of the last 10 lines (`tail` in the source). Status is derived
entirely from `tail`'s single most recent line — the line that is on screen
right now, not from scanning the whole window.

This is deliberate, and it fixes a real false positive: an earlier version
tested every line in the 10-line window for an activity indicator anywhere
in its text. A finished status banner from a completed command — for
example a test runner printing `Running 12 tests` on its way to a `PASS` —
would sit in that window and read as "active" for several scan cycles after
the shell had already returned to an idle prompt. Status must reflect
*now*, so only the last line is tested.

## Active indicators

`activeIndicatorPattern` matches a spinner/status word anchored at the
start of the last line, optionally preceded by whitespace or one of Claude
Code's spinner glyphs (`⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏·✻✽✶✳✢*`):

```
^[\s⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏·✻✽✶✳✢*]*(?:Thinking|Reading|Writing|Editing|Running|Searching|Analyzing)\b
```

If the last line matches, status is `active`. Anchoring at the start of the
line (rather than a substring match anywhere in it) means a status word
that shows up mid-sentence elsewhere in the line's text does not trigger a
false positive.

## Prompt patterns (idle)

If the last line does not match an active indicator, it is checked against
the idle prompt patterns:

```
\$\s*$        # shell prompt
>\s*$         # generic prompt
claude>\s*$   # Claude Code REPL prompt
\?\s+$        # Claude Code question prompt
```

A match means `idle`. If neither the active nor the idle pattern matches,
status is `unknown` (this is also the default for an empty pane).

## Bead, git, and test patterns

Independently of status, `ParsePaneContent` scans every captured line (not
just the trailing window) for:

- **Bead IDs** (`extractBeadIDs`): `bd show/close/update/note/reopen/claim/start/done <id>`, `bd dep` lines, status-glyph lines (`○ ◐ ● ✓ ↳ ↑ ←`) that also carry a `·`, `—`, or "issue:" marker, and explicit `bead:`/`beads:`/`issue:`/`issues:` references. A generic hyphenated shell term (`git-push`, `hot-fix`) is not treated as a bead ID unless it appears in one of these contexts.
- **File edits** (`fileEditPattern`): `Edit`, `Write`, or `Read` followed by an absolute path, feeding `FilesTouched`.
- **Git and test activity** (`DetectEventType`, used by the watcher to log activity events): `git commit`/`committed`/`create mode` → `git_commit`; `git push`/`Enumerating objects`/`Writing objects` → `git_push`; `go test`/`pytest`/`npm test`/`PASS`/`FAIL` → `test_run`; a file edit → `file_edit`; otherwise `command_run`.

## Stuck and crashed

Neither of these come from `ParsePaneContent` — both require state across
scans:

- **Stuck**: the watcher (`watcher.go`) tracks when each session's captured content last changed. If status is `active` but the content hasn't changed for more than 5 minutes, status is escalated to `stuck`. `internal/health/monitor.go` runs a second, coarser sweep on the same 5-minute threshold (`MonitorConfig.StuckTimeout`, default `DefaultMonitorConfig()`): any agent whose `LastSeen` timestamp is more than `StuckTimeout` old while its stored status is still `active` gets marked `stuck` too. This is a backstop for agents the per-scan check missed (e.g. the process pushing metadata stalled).
- **Crashed**: the watcher checks whether the pane's process (`PanePID`) is still alive (`processAlive`, a `syscall.Kill(pid, 0)` existence probe — chosen over reading `/proc` because `/proc` doesn't exist on macOS). If it isn't, status is `crashed` regardless of what the pane content says.

## Idle threshold (server-level, distinct from agent idle status)

Separately from an *agent's* idle status above, the intermux-mcp process
itself backs off its own background goroutines (tmux scan, health check,
metadata push, mapping-file watch) when no MCP tool call has been received
for 60 seconds (`idle.NewTracker(60 * time.Second)` in `cmd/intermux-mcp/main.go`).
This only affects how often intermux polls tmux — it has no bearing on the
active/idle/stuck/crashed classification of the agents it observes.

## Known false positives

- **Fixed**: a completed status word (e.g. `Running`) earlier in the
  10-line window used to be read as still-active even after a shell prompt
  showed the agent had returned to idle. Fixed by testing only the last
  line (see above); `internal/tmux/parser_status_test.go` covers this case.
- **Still open**: any literal occurrence of a bare indicator word as the
  first token of the pane's current last line reads as active, even if a
  human wrote it in prose rather than a tool actually running (e.g. an
  agent's own last output line starting with the word "Running" as normal
  English, not a spinner). This is a heuristic over free text; it has no
  ground truth to check against.
- Bead ID extraction can still mis-attribute an ID mentioned in passing
  (e.g. while discussing a different session's work) as this session's
  active bead; `active_bead_confidence` (see
  `docs/presence-metadata-contract.md`) exists to signal when more than
  one candidate was seen.
