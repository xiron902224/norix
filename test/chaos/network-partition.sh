#!/usr/bin/env bash
# Chaos: disconnect agent from compose network → offline; reconnect → online.
# Uses docker network disconnect (no host iptables). Expected: docs/user/chaos.md
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

NET="$("${COMPOSE[@]}" ps -q control-plane | xargs -I{} docker inspect -f '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}' {})"
[[ -n "$NET" ]] || chaos_fail "could not resolve compose network"

chaos_log "disconnecting norix-mn-agent-2 from $NET"
docker network disconnect "$NET" norix-mn-agent-2 || chaos_fail "disconnect failed"
wait_node_status "$SESSION" "$NODE_ID" "offline" 120 >/dev/null
chaos_log "agent offline during partition"

chaos_log "reconnecting agent-2"
docker network connect "$NET" norix-mn-agent-2
# Agent process may still be running and resume heartbeats.
wait_node_status "$SESSION" "$NODE_ID" "online" 180 >/dev/null
chaos_log "OK network-partition: offline then online again"
