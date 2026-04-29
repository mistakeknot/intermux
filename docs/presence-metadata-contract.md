# Intermux Presence Metadata Contract

Status: implemented v0 shim for `sylveste-kgfi.1`.

Alignment: supports intermux's activity-visibility purpose by turning tmux observation into a compact coordination record that other agents can query through existing intermute agent metadata.

Conflict/Risk: presence is observational and may be stale or ambiguous; Beads remains canonical for task state, priority, dependencies, and claim/assignment.

## Intermute metadata keys

Intermux publishes these string-valued keys to `PATCH /api/agents/{agent_id}/metadata`:

| Key | Meaning | Confidence/source |
| --- | --- | --- |
| `tmux_session` | Stable observed session identity, e.g. `ghostty-sylveste-claude` | tmux session name |
| `agent_kind` | Parsed agent kind, e.g. `claude`, `codex`, `dev` | session-name parser |
| `project` | Parsed project slug from session name, when available | session-name parser |
| `repo` | Resolved git root / project directory from the pane process cwd | `/proc/<pid>/cwd` + git root walk |
| `cwd` | Current pane process working directory | `/proc/<pid>/cwd` |
| `git_branch` | Current branch or detached HEAD prefix | `.git/HEAD` walk from cwd |
| `status` | `active`, `idle`, `stuck`, `crashed`, or `unknown` | pane/status heuristics |
| `active_bead_id` | Best current Beads join key when confidence is sufficient; empty string clears stale merged metadata when unknown/ambiguous | reported or single observed candidate |
| `active_bead_confidence` | `reported`, `observed`, or `unknown` in v0 | see confidence rules below |
| `active_beads` | JSON array of all observed candidate bead IDs | pane content with Beads context |
| `active_bead_candidates` | JSON array emitted only when multiple candidates prevent a safe singular `active_bead_id` | ambiguity-preserving fallback |
| `thread_id` | Defaults to `active_bead_id` when a singular active bead is known | join handle for message threads |
| `files_touched` | JSON array of observed files from pane output | Claude/Codex file-action output |
| `last_activity` | Last meaningful line from pane output | pane parser |
| `last_activity_at` | RFC3339 timestamp, retained for backward compatibility | watcher observation time |
| `last_seen` | RFC3339 timestamp for the presence record | watcher observation time |

## Confidence rules

1. Explicit launch/session metadata wins:
   - SessionStart hook accepts `INTERMUX_ACTIVE_BEAD_ID`, `ACTIVE_BEAD_ID`, or `BEAD_ID` and writes it to `/tmp/intermux-mapping-*.json`.
   - Mapping ingestion merges it into `agent.Metadata["active_bead_id"]`.
   - confidence defaults to `reported` unless `INTERMUX_ACTIVE_BEAD_CONFIDENCE` / `agent.Metadata["active_bead_confidence"]` is set.
2. A single Beads-context candidate from pane output becomes `active_bead_id` with confidence `observed`.
3. Multiple candidates are **not** guessed. Intermux sends `active_bead_id=""`, `thread_id=""`, `active_bead_confidence=unknown`, and publishes `active_bead_candidates`.
4. No candidates also yields `active_bead_id=""`, `thread_id=""`, `active_bead_confidence=unknown`, `active_beads=[]`, `active_bead_candidates=[]`, and `files_touched=[]` so merge-only metadata PATCH receivers do not preserve stale presence.

## Example metadata

```json
{
  "tmux_session": "ghostty-sylveste-claude",
  "agent_kind": "claude",
  "project": "sylveste",
  "repo": "/home/mk/projects/Sylveste",
  "cwd": "/home/mk/projects/Sylveste/interverse/intermux",
  "git_branch": "main",
  "status": "active",
  "active_bead_id": "sylveste-kgfi.1",
  "active_bead_confidence": "observed",
  "active_beads": "[\"sylveste-kgfi.1\"]",
  "thread_id": "sylveste-kgfi.1",
  "files_touched": "[\"internal/tmux/parser.go\",\"internal/push/pusher.go\"]",
  "last_seen": "2026-04-29T03:45:00Z"
}
```

## Session-kind fixture notes

Intermux distinguishes common live agent bodies through the existing session-name parser:

| Session name | Parsed kind | Notes |
| --- | --- | --- |
| `ghostty-sylveste-claude` | `claude` | Claude Code-style tmux session |
| `alacritty-agmodb-codex` | `codex` | Codex-style tmux session |
| `rio-autarch-dev` | `dev` | generic/dev agent session |

Hermes-native Discord sessions are not necessarily tmux-backed; Athenmesh should later merge this metadata with Hermes/CASS/Beads state rather than requiring Hermes to appear as a tmux pane.

## Verification

Targeted tests:

```bash
/usr/local/go/bin/go test ./internal/tmux ./internal/push
```

Full plugin verification:

```bash
/usr/local/go/bin/go test ./...
/usr/local/go/bin/go build -o bin/intermux-mcp ./cmd/intermux-mcp/
python3 -c "import json; json.load(open('.claude-plugin/plugin.json'))"
bash -n hooks/*.sh
```
