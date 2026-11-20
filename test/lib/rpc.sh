#!/usr/bin/env bash
# Connect-RPC JSON helpers for Norix smokes (multinode / chaos).
# Requires: curl, jq. Sets NORIX_CP_URL (default http://127.0.0.1:18080).

: "${NORIX_CP_URL:=http://127.0.0.1:18080}"

norix_rpc() {
  local procedure="$1"
  local body="${2:-{}}"
  local token="${3:-}"
  local url="${NORIX_CP_URL%/}/${procedure#/}"
  local args=(
    -sS
    -X POST
    -H "Content-Type: application/json"
    -H "Connect-Protocol-Version: 1"
    --data "$body"
  )
  if [[ -n "$token" ]]; then
    args+=(-H "Authorization: Bearer ${token}")
  fi
  curl "${args[@]}" "$url"
}

norix_rpc_ok() {
  local procedure="$1"
  local body="${2:-{}}"
  local token="${3:-}"
  local resp
  resp="$(norix_rpc "$procedure" "$body" "$token")" || {
    echo "error: RPC transport failed: $procedure" >&2
    return 1
  }
  if echo "$resp" | jq -e 'has("code")' >/dev/null 2>&1; then
    echo "error: RPC $procedure → $resp" >&2
    return 1
  fi
  printf '%s' "$resp"
}

wait_http_ok() {
  local url="$1"
  local timeout_s="${2:-120}"
  local i=0
  while (( i < timeout_s )); do
    if curl -sf "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
    i=$((i + 1))
  done
  echo "error: timed out waiting for $url (${timeout_s}s)" >&2
  return 1
}

wait_nodes_online() {
  local token="$1"
  local want="$2"
  local timeout_s="${3:-180}"
  local i=0
  while (( i < timeout_s )); do
    local resp online
    resp="$(norix_rpc_ok "v1.AgentService/ListNodes" "{}" "$token")" || true
    online="$(echo "$resp" | jq '[.nodes[]? | select(.status == "online")] | length' 2>/dev/null || echo 0)"
    if [[ "$online" -ge "$want" ]]; then
      echo "$resp"
      return 0
    fi
    sleep 2
    i=$((i + 2))
  done
  echo "error: timed out waiting for $want online nodes" >&2
  norix_rpc "v1.AgentService/ListNodes" "{}" "$token" >&2 || true
  return 1
}

wait_node_status() {
  local token="$1"
  local node_id="$2"
  local want_status="$3"
  local timeout_s="${4:-120}"
  local i=0
  while (( i < timeout_s )); do
    local resp status
    resp="$(norix_rpc_ok "v1.AgentService/ListNodes" "{}" "$token")" || true
    status="$(echo "$resp" | jq -r --arg id "$node_id" '.nodes[]? | select(.id == $id) | .status' 2>/dev/null || true)"
    if [[ "$status" == "$want_status" ]]; then
      echo "$resp"
      return 0
    fi
    sleep 2
    i=$((i + 2))
  done
  echo "error: node $node_id did not become $want_status within ${timeout_s}s (last=$status)" >&2
  return 1
}
