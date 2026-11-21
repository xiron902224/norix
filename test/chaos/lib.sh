#!/usr/bin/env bash
# Shared helpers for chaos scenarios (#41). Reuses the multinode compose stack.
set -euo pipefail

CHAOS_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$CHAOS_ROOT/../.." && pwd)"
MN_DIR="$REPO_ROOT/test/multinode"
# shellcheck source=../lib/rpc.sh
source "$REPO_ROOT/test/lib/rpc.sh"

export NORIX_CP_URL="${NORIX_CP_URL:-http://127.0.0.1:18080}"
COMPOSE=(docker compose --env-file "$MN_DIR/.env" -f "$MN_DIR/docker-compose.yml")

ADMIN_EMAIL="${NORIX_MN_ADMIN_EMAIL:-mn-admin@norix.test}"
ADMIN_PASSWORD="${NORIX_MN_ADMIN_PASSWORD:-MultinodeSmoke1!}"

# stderr only — callers capture chaos_session / setup output as tokens.
chaos_log() { printf '[chaos] %s\n' "$*" >&2; }
chaos_fail() { echo "error: $*" >&2; exit 1; }

# Bring up postgres+CP+N agents. Leaves stack running.
chaos_setup() {
  local agents="${1:-2}"
  command -v jq >/dev/null || chaos_fail "jq required"
  mkdir -p "$MN_DIR/bin"
  if [[ ! -x "$MN_DIR/bin/norix-control-plane" ]]; then
    mkdir -p "$REPO_ROOT/control-plane/internal/webui/dist"
    [[ -f "$REPO_ROOT/control-plane/internal/webui/dist/index.html" ]] \
      || echo '<!doctype html><title>chaos</title>' >"$REPO_ROOT/control-plane/internal/webui/dist/index.html"
    (cd "$REPO_ROOT/control-plane" && CGO_ENABLED=0 go build -o "$MN_DIR/bin/norix-control-plane" ./cmd/server)
  fi
  if [[ ! -x "$MN_DIR/bin/norix-agent" ]]; then
    (cd "$REPO_ROOT/agent" && cargo build --release -q)
    cp "$REPO_ROOT/agent/target/release/norix-agent" "$MN_DIR/bin/norix-agent"
  fi
  chmod +x "$MN_DIR/bin/norix-control-plane" "$MN_DIR/bin/norix-agent"
  export NORIX_CP_BIN="$MN_DIR/bin/norix-control-plane"
  export NORIX_AGENT_BIN="$MN_DIR/bin/norix-agent"

  local key
  key="$(openssl rand -hex 32)"
  cat >"$MN_DIR/.env" <<EOF
POSTGRES_DB=norix
POSTGRES_USER=norix
POSTGRES_PASSWORD=multinode-ci-postgres
POSTGRES_APP_PASSWORD=multinode-ci-app
DATABASE_URL=postgres://norix_app:multinode-ci-app@postgres:5432/norix?sslmode=disable
MIGRATION_DATABASE_URL=postgres://norix:multinode-ci-postgres@postgres:5432/norix?sslmode=disable
NORIX_HTTP_PORT=18080
NORIX_PUBLIC_URL=http://127.0.0.1:18080
NORIX_WEB_ORIGIN=http://127.0.0.1:18080
CORS_ALLOWED_ORIGINS=http://127.0.0.1:18080
NORIX_BUILD_SECRETS_KEY=${key}
NORIX_CP_BIN=${NORIX_CP_BIN}
NORIX_AGENT_BIN=${NORIX_AGENT_BIN}
AGENT1_JOIN_TOKEN=
AGENT2_JOIN_TOKEN=
AGENT3_JOIN_TOKEN=
EOF

  "${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
  "${COMPOSE[@]}" up -d --build postgres control-plane
  wait_http_ok "${NORIX_CP_URL}/healthz" 180

  local status token
  status="$(norix_rpc_ok "v1.AuthService/GetBootstrapStatus" "{}")"
  if [[ "$(echo "$status" | jq -r '.configured')" == "true" ]]; then
    token="$(norix_rpc_ok "v1.AuthService/Login" \
      "$(jq -nc --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" '{email:$e,password:$p}')" | jq -r '.token')"
  else
    token="$(norix_rpc_ok "v1.AuthService/CreateInitialAdmin" \
      "$(jq -nc --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" \
        '{organizationName:"chaos-lab",adminName:"Chaos Admin",adminEmail:$e,adminPassword:$p}')" | jq -r '.token')"
  fi
  export CHAOS_SESSION="$token"

  local t1 t2 t3
  t1="$(norix_rpc_ok "v1.AgentService/CreateJoinToken" '{"nodeName":"agent-1","ttlSeconds":3600}' "$token" | jq -r '.token')"
  t2="$(norix_rpc_ok "v1.AgentService/CreateJoinToken" '{"nodeName":"agent-2","ttlSeconds":3600}' "$token" | jq -r '.token')"
  t3="$(norix_rpc_ok "v1.AgentService/CreateJoinToken" '{"nodeName":"agent-3","ttlSeconds":3600}' "$token" | jq -r '.token')"
  # Portable in-place edit (GNU/BSD sed).
  local tmp
  tmp="$(mktemp)"
  awk -v a1="$t1" -v a2="$t2" -v a3="$t3" '
    /^AGENT1_JOIN_TOKEN=/ { print "AGENT1_JOIN_TOKEN=" a1; next }
    /^AGENT2_JOIN_TOKEN=/ { print "AGENT2_JOIN_TOKEN=" a2; next }
    /^AGENT3_JOIN_TOKEN=/ { print "AGENT3_JOIN_TOKEN=" a3; next }
    { print }
  ' "$MN_DIR/.env" >"$tmp" && mv "$tmp" "$MN_DIR/.env"

  case "$agents" in
    1) "${COMPOSE[@]}" up -d --build agent-1 ;;
    2) "${COMPOSE[@]}" up -d --build agent-1 agent-2 ;;
    *) "${COMPOSE[@]}" up -d --build agent-1 agent-2 agent-3 ;;
  esac
  wait_nodes_online "$token" "$agents" 180 >/dev/null
  chaos_log "stack ready ($agents agents)"
}

chaos_teardown() {
  if [[ "${NORIX_CHAOS_KEEP:-0}" == "1" ]]; then
    chaos_log "NORIX_CHAOS_KEEP=1 — leaving stack"
    return 0
  fi
  "${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
}

chaos_session() {
  if [[ -n "${CHAOS_SESSION:-}" ]]; then
    printf '%s' "$CHAOS_SESSION"
    return 0
  fi
  norix_rpc_ok "v1.AuthService/Login" \
    "$(jq -nc --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" '{email:$e,password:$p}')" | jq -r '.token'
}

node_id_by_name() {
  local session="$1" name="$2"
  norix_rpc_ok "v1.AgentService/ListNodes" "{}" "$session" \
    | jq -r --arg n "$name" '.nodes[] | select(.name==$n) | .id' | head -1
}
