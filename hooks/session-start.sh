#!/usr/bin/env bash
# Write a mapping file so intermux can correlate tmux sessions with intermute agent IDs.
set -uo pipefail
trap 'exit 0' ERR

INPUT=$(cat)
SID=$(echo "$INPUT" | jq -r '.session_id // empty' 2>/dev/null) || exit 0
[[ -n "$SID" ]] || exit 0

TMUX_SESSION=""
if [[ -n "${TMUX:-}" ]]; then
  TMUX_SESSION=$(tmux display-message -p '#{session_name}' 2>/dev/null) || true
fi

AGENT_ID="${INTERMUTE_AGENT_ID:-}"
ACTIVE_BEAD_ID="${INTERMUX_ACTIVE_BEAD_ID:-${ACTIVE_BEAD_ID:-${BEAD_ID:-}}}"
ACTIVE_BEAD_CONFIDENCE=""
if [[ -n "$ACTIVE_BEAD_ID" ]]; then
  ACTIVE_BEAD_CONFIDENCE="${INTERMUX_ACTIVE_BEAD_CONFIDENCE:-reported}"
fi

jq -n \
  --arg sid "$SID" \
  --arg tmux "$TMUX_SESSION" \
  --arg aid "$AGENT_ID" \
  --arg active_bead_id "$ACTIVE_BEAD_ID" \
  --arg active_bead_confidence "$ACTIVE_BEAD_CONFIDENCE" \
  '{session_id:$sid, tmux_session:$tmux, agent_id:$aid, active_bead_id:$active_bead_id, active_bead_confidence:$active_bead_confidence}' \
  > "/tmp/intermux-mapping-${SID}.json" 2>/dev/null || true

exit 0
