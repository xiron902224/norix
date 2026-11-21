#!/usr/bin/env bash
# Chaos: agent crash → node offline (heartbeat stale).
# Expected: see docs/user/chaos.md
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib.sh
source "$DIR/lib.sh"

OWN_SETUP=0
if [[ "${1:-}" == "--setup" ]] || ! curl -sf "${NORIX_CP_URL}/healthz" >/dev/null 2>&1; then
  chaos_setup 2
  OWN_SETUP=1
  trap '[[ "$OWN_SETUP" == "1" ]] && chaos_teardown' EXIT
fi

SESSION="$(chaos_session)"
NODE_ID="$(node_id_by_name "$SESSION" "agent-2")"
[[ -n "$NODE_ID" && "$NODE_ID" != "null" ]] || chaos_fail "agent-2 not found"

chaos_log "killing norix-mn-agent-2"
docker kill norix-mn-agent-2 >/dev/null
wait_node_status "$SESSION" "$NODE_ID" "offline" 120 >/dev/null
chaos_log "OK agent-crash: node offline"
