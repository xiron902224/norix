#!/usr/bin/env bash
# Chaos: Postgres restart → CP recovers (may need CP restart if pool is wedged).
# Expected: see docs/user/chaos.md
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib.sh
source "$DIR/lib.sh"

OWN_SETUP=0
if [[ "${1:-}" == "--setup" ]] || ! curl -sf "${NORIX_CP_URL}/healthz" >/dev/null 2>&1; then
  chaos_setup 1
  OWN_SETUP=1
  trap '[[ "$OWN_SETUP" == "1" ]] && chaos_teardown' EXIT
fi

chaos_log "restarting postgres"
"${COMPOSE[@]}" restart postgres
# Wait for PG health, then CP healthz (restart CP if needed).
local_i=0
while (( local_i < 90 )); do
  if "${COMPOSE[@]}" exec -T postgres pg_isready -U norix -d norix >/dev/null 2>&1; then
    break
  fi
  sleep 2
  local_i=$((local_i + 2))
done

if ! curl -sf "${NORIX_CP_URL}/healthz" >/dev/null 2>&1; then
  chaos_log "CP unhealthy after DB blip — restarting control-plane"
  "${COMPOSE[@]}" restart control-plane
fi
wait_http_ok "${NORIX_CP_URL}/healthz" 120

unset CHAOS_SESSION
SESSION="$(chaos_session)"
wait_nodes_online "$SESSION" 1 180 >/dev/null
chaos_log "OK db-restart: healthz + ≥1 online agent"
