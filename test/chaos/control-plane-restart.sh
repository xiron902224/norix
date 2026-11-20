#!/usr/bin/env bash
# Chaos: control-plane container restart → healthz returns; agents stay/rejoin online.
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
BEFORE="$(norix_rpc_ok "v1.AgentService/ListNodes" "{}" "$SESSION" \
  | jq '[.nodes[] | select(.status=="online")] | length')"

chaos_log "restarting control-plane"
"${COMPOSE[@]}" restart control-plane
wait_http_ok "${NORIX_CP_URL}/healthz" 120

# Session may still be valid (DB-backed). Re-login if needed.
if ! norix_rpc_ok "v1.AgentService/ListNodes" "{}" "$SESSION" >/dev/null 2>&1; then
  unset CHAOS_SESSION
  SESSION="$(chaos_session)"
fi

wait_nodes_online "$SESSION" "$BEFORE" 180 >/dev/null
chaos_log "OK control-plane-restart: healthz + $BEFORE online agents"
