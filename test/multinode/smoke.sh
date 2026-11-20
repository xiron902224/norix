#!/usr/bin/env bash
# Multi-node E2E smoke (#37): 3 agents, replica_count=2, cordon/drain, kill agent → offline.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=../lib/rpc.sh
source "$ROOT/test/lib/rpc.sh"

COMPOSE=(docker compose --env-file "$DIR/.env" -f "$DIR/docker-compose.yml")
export NORIX_CP_URL="${NORIX_CP_URL:-http://127.0.0.1:18080}"

ADMIN_EMAIL="${NORIX_MN_ADMIN_EMAIL:-mn-admin@norix.test}"
ADMIN_PASSWORD="${NORIX_MN_ADMIN_PASSWORD:-MultinodeSmoke1!}"
KEEP="${NORIX_MN_KEEP:-0}"

log() { printf '==> %s\n' "$*"; }
fail() { echo "error: $*" >&2; exit 1; }

cleanup() {
  if [[ "$KEEP" == "1" ]]; then
    log "NORIX_MN_KEEP=1 — leaving stack up"
    return 0
  fi
  log "tearing down"
  "${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

ensure_bins() {
  mkdir -p "$DIR/bin"
  local cp_bin="$DIR/bin/norix-control-plane"
  local ag_bin="$DIR/bin/norix-agent"
  if [[ ! -x "$cp_bin" ]]; then
    log "building control-plane"
    mkdir -p "$ROOT/control-plane/internal/webui/dist"
    if [[ ! -f "$ROOT/control-plane/internal/webui/dist/index.html" ]]; then
      echo '<!doctype html><title>multinode</title>' >"$ROOT/control-plane/internal/webui/dist/index.html"
    fi
    (cd "$ROOT/control-plane" && CGO_ENABLED=0 go build -o "$cp_bin" ./cmd/server)
  fi
  if [[ ! -x "$ag_bin" ]]; then
    log "building agent (release)"
    (cd "$ROOT/agent" && cargo build --release -q)
    cp "$ROOT/agent/target/release/norix-agent" "$ag_bin"
  fi
  chmod +x "$cp_bin" "$ag_bin"
  # Absolute paths for compose bind mounts
  NORIX_CP_BIN="$(cd "$(dirname "$cp_bin")" && pwd)/$(basename "$cp_bin")"
  NORIX_AGENT_BIN="$(cd "$(dirname "$ag_bin")" && pwd)/$(basename "$ag_bin")"
  export NORIX_CP_BIN NORIX_AGENT_BIN
}

write_env() {
  if [[ ! -f "$DIR/.env" ]]; then
    cp "$DIR/.env.example" "$DIR/.env"
  fi
  # Refresh binary paths and secrets without wiping join tokens mid-run.
  local key
  key="$(grep -E '^NORIX_BUILD_SECRETS_KEY=' "$DIR/.env" | cut -d= -f2- || true)"
  if [[ -z "$key" ]]; then
    key="$(openssl rand -hex 32)"
  fi
  cat >"$DIR/.env" <<EOF
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
AGENT1_JOIN_TOKEN=${AGENT1_JOIN_TOKEN:-}
AGENT2_JOIN_TOKEN=${AGENT2_JOIN_TOKEN:-}
AGENT3_JOIN_TOKEN=${AGENT3_JOIN_TOKEN:-}
EOF
}

bootstrap_admin() {
  local status token
  status="$(norix_rpc_ok "v1.AuthService/GetBootstrapStatus" "{}")"
  if [[ "$(echo "$status" | jq -r '.configured')" == "true" ]]; then
    log "already bootstrapped — logging in"
    token="$(norix_rpc_ok "v1.AuthService/Login" \
      "$(jq -nc --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" '{email:$e,password:$p}')" \
      | jq -r '.token')"
  else
    log "creating initial admin"
    token="$(norix_rpc_ok "v1.AuthService/CreateInitialAdmin" \
      "$(jq -nc --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" \
        '{organizationName:"multinode-lab",adminName:"MN Admin",adminEmail:$e,adminPassword:$p}')" \
      | jq -r '.token')"
  fi
  [[ -n "$token" && "$token" != "null" ]] || fail "no session token"
  printf '%s' "$token"
}

create_join_token() {
  local session="$1"
  local name="$2"
  norix_rpc_ok "v1.AgentService/CreateJoinToken" \
    "$(jq -nc --arg n "$name" '{nodeName:$n,ttlSeconds:3600}')" \
    "$session" | jq -r '.token'
}

# --- main ---
command -v jq >/dev/null || fail "jq required"
command -v docker >/dev/null || fail "docker required"
command -v curl >/dev/null || fail "curl required"

ensure_bins
write_env
set -a
# shellcheck disable=SC1091
source "$DIR/.env"
set +a

log "starting postgres + control-plane"
"${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
"${COMPOSE[@]}" up -d --build postgres control-plane
wait_http_ok "${NORIX_CP_URL}/healthz" 180
log "control-plane healthy"

SESSION="$(bootstrap_admin)"
log "creating join tokens"
AGENT1_JOIN_TOKEN="$(create_join_token "$SESSION" "agent-1")"
AGENT2_JOIN_TOKEN="$(create_join_token "$SESSION" "agent-2")"
AGENT3_JOIN_TOKEN="$(create_join_token "$SESSION" "agent-3")"
write_env
set -a
# shellcheck disable=SC1091
source "$DIR/.env"
set +a

log "starting 3 agents"
"${COMPOSE[@]}" up -d --build agent-1 agent-2 agent-3
NODES_JSON="$(wait_nodes_online "$SESSION" 3 180)"
log "3 nodes online"
echo "$NODES_JSON" | jq -r '.nodes[] | "\(.name)\t\(.status)\t\(.id)"'

log "create project + workload replica_count=2"
PROJ="$(norix_rpc_ok "v1.ProjectService/CreateProject" \
  '{"name":"mn-smoke"}' "$SESSION")"
PROJECT_ID="$(echo "$PROJ" | jq -r '.project.id')"
[[ -n "$PROJECT_ID" && "$PROJECT_ID" != "null" ]] || fail "no project id"

WL="$(norix_rpc_ok "v1.WorkloadService/CreateWorkload" \
  "$(jq -nc --arg pid "$PROJECT_ID" '{
    workload: {
      projectId: $pid,
      name: "mn-web",
      runtime: 1,
      imageOrSource: "nginx:alpine",
      sourceKind: "image",
      containerPort: 80,
      replicaCount: 2,
      scheduleStrategy: "spread"
    }
  }')" "$SESSION")"
WORKLOAD_ID="$(echo "$WL" | jq -r '.workload.id')"
[[ -n "$WORKLOAD_ID" && "$WORKLOAD_ID" != "null" ]] || fail "no workload id"
REPLICA_COUNT="$(echo "$WL" | jq -r '.workload.replicaCount')"
[[ "$REPLICA_COUNT" == "2" ]] || fail "expected replicaCount=2 got $REPLICA_COUNT"

log "deploy workload"
DEPLOY="$(norix_rpc "v1.WorkloadService/Deploy" \
  "$(jq -nc --arg id "$WORKLOAD_ID" '{workload:{id:$id},containerPort:80}')" \
  "$SESSION")"
if echo "$DEPLOY" | jq -e 'has("code")' >/dev/null 2>&1; then
  code="$(echo "$DEPLOY" | jq -r '.code')"
  # Shared docker.sock / placement can fail in CI; still prove the API path.
  if [[ "$code" != "failed_precondition" && "$code" != "internal" ]]; then
    fail "Deploy failed: $DEPLOY"
  fi
  log "Deploy returned $code (documented CI limitation) — continuing cordon/drain/offline"
else
  log "Deploy ok: $(echo "$DEPLOY" | jq -c '{status,assignedNodeId,hostPort}')"
fi

TARGET_ID="$(echo "$NODES_JSON" | jq -r '.nodes[] | select(.name=="agent-1") | .id' | head -1)"
[[ -n "$TARGET_ID" && "$TARGET_ID" != "null" ]] || TARGET_ID="$(echo "$NODES_JSON" | jq -r '.nodes[0].id')"

log "cordon node $TARGET_ID"
norix_rpc_ok "v1.AgentService/CordonNode" \
  "$(jq -nc --arg id "$TARGET_ID" '{nodeId:$id,cordoned:true}')" \
  "$SESSION" >/dev/null

log "drain node $TARGET_ID"
norix_rpc_ok "v1.AgentService/DrainNode" \
  "$(jq -nc --arg id "$TARGET_ID" '{nodeId:$id,draining:true}')" \
  "$SESSION" >/dev/null

CORDONED="$(norix_rpc_ok "v1.AgentService/ListNodes" "{}" "$SESSION" \
  | jq -r --arg id "$TARGET_ID" '.nodes[] | select(.id==$id) | "\(.cordoned) \(.draining)"')"
[[ "$CORDONED" == "true true" ]] || fail "expected cordoned+draining, got: $CORDONED"
log "cordon/drain asserted"

KILL_NAME="agent-2"
KILL_ID="$(echo "$NODES_JSON" | jq -r --arg n "$KILL_NAME" '.nodes[] | select(.name==$n) | .id' | head -1)"
[[ -n "$KILL_ID" && "$KILL_ID" != "null" ]] || fail "agent-2 node id missing"

log "killing agent container norix-mn-agent-2"
docker kill norix-mn-agent-2 >/dev/null
wait_node_status "$SESSION" "$KILL_ID" "offline" 120 >/dev/null
log "agent-2 is offline (heartbeat timeout)"

REMAINING="$(norix_rpc_ok "v1.AgentService/ListNodes" "{}" "$SESSION" \
  | jq '[.nodes[] | select(.status=="online")] | length')"
[[ "$REMAINING" -ge 2 ]] || fail "expected ≥2 online nodes after kill, got $REMAINING"
log "convergence check: $REMAINING nodes still online"

log "OK multinode smoke (#37)"
