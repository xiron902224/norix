#!/usr/bin/env bash
# Norix self-host installer (Coolify / OpenShip-shaped).
#
# Until get.norix.dev exists, install from the GitHub repo:
#
#   curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/develop/scripts/get-norix.sh | bash
#
# Pin a branch / tag:
#   curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/develop/scripts/get-norix.sh \
#     | NORIX_REF=v1.0.0-beta.1 bash
#
# From a local checkout:
#   sudo ./scripts/get-norix.sh
#
# Env:
#   NORIX_REF          git ref to clone (default: develop)
#   NORIX_REPO         git URL (default: https://github.com/xiron902224/norix.git)
#   NORIX_DATA         data root (default: /data/norix)
#   NORIX_HTTP_PORT    panel port (default: 8000)
#   NORIX_PUBLIC_URL   public URL printed / written to .env (auto-detected if empty)
#   NORIX_SKIP_DOCKER  if 1, do not install Docker (must already exist)
#   NORIX_PULL_POLICY  build|missing|always (default: build until GHCR images ship)

set -euo pipefail

NORIX_REPO="${NORIX_REPO:-https://github.com/xiron902224/norix.git}"
# Default tip; pin with NORIX_REF=v1.0.0-beta.1 for the beta release.
NORIX_REF="${NORIX_REF:-develop}"
NORIX_DATA="${NORIX_DATA:-/data/norix}"
NORIX_HTTP_PORT="${NORIX_HTTP_PORT:-8000}"
NORIX_SKIP_DOCKER="${NORIX_SKIP_DOCKER:-0}"
NORIX_PULL_POLICY="${NORIX_PULL_POLICY:-build}"
NORIX_GITHUB_RAW="${NORIX_GITHUB_RAW:-https://raw.githubusercontent.com/xiron902224/norix}"

SOURCE_DIR="${NORIX_DATA}/source"
ENV_FILE="${SOURCE_DIR}/.env"
COMPOSE_FILE="deploy/norix/docker-compose.prod.yml"

log() { printf '\033[36m==>\033[0m %s\n' "$*"; }
ok() { printf '\033[32m✔\033[0m %s\n' "$*"; }
die() { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Usage: get-norix.sh [--help]

Installs Norix all-in-one on this Linux host (Postgres + control plane + docker.sock).

  curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/develop/scripts/get-norix.sh | bash

Options / env: see script header.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ "$(id -u)" -ne 0 ]]; then
  die "run as root (or: curl … | sudo bash)"
fi

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v git >/dev/null 2>&1 || {
  log "Installing git…"
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -y && apt-get install -y git
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y git
  else
    die "git is required"
  fi
}

detect_public_url() {
  if [[ -n "${NORIX_PUBLIC_URL:-}" ]]; then
    printf '%s' "$NORIX_PUBLIC_URL"
    return
  fi
  local ip=""
  ip="$(curl -fsS --max-time 3 https://ifconfig.me 2>/dev/null || true)"
  if [[ -z "$ip" ]]; then
    ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  fi
  if [[ -z "$ip" ]]; then
    ip="127.0.0.1"
  fi
  printf 'http://%s:%s' "$ip" "$NORIX_HTTP_PORT"
}

install_docker() {
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    ok "Docker Engine + Compose already present ($(docker version --format '{{.Server.Version}}' 2>/dev/null || echo ok))"
    return
  fi
  [[ "$NORIX_SKIP_DOCKER" == "1" ]] && die "Docker/Compose missing and NORIX_SKIP_DOCKER=1"
  log "Installing Docker Engine (get.docker.com)…"
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker 2>/dev/null || true
  command -v docker >/dev/null 2>&1 || die "Docker install failed"
  docker compose version >/dev/null 2>&1 || die "Docker Compose v2 plugin missing"
  ok "Docker installed"
}

sync_source() {
  mkdir -p "$NORIX_DATA"/{backups,agent}
  chmod 700 "$NORIX_DATA"

  # Running from a local checkout that already has compose.prod?
  local here
  here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." 2>/dev/null && pwd || true)"
  if [[ -n "$here" && -f "$here/$COMPOSE_FILE" && -f "$here/control-plane/Dockerfile.prod" ]]; then
    log "Using local repository at $here"
    mkdir -p "$SOURCE_DIR"
    # Keep a working copy under /data/norix/source for upgrades (rsync-like).
    if command -v rsync >/dev/null 2>&1; then
      rsync -a --delete \
        --exclude '.git' --exclude '**/node_modules' --exclude 'agent/target' \
        --exclude 'control-plane/bin' --exclude 'web/dist' \
        "$here/" "$SOURCE_DIR/"
    else
      rm -rf "${SOURCE_DIR}.tmp"
      mkdir -p "${SOURCE_DIR}.tmp"
      tar -C "$here" \
        --exclude='.git' --exclude='node_modules' --exclude='agent/target' \
        --exclude='control-plane/bin' --exclude='web/dist' \
        -cf - . | tar -C "${SOURCE_DIR}.tmp" -xf -
      rm -rf "$SOURCE_DIR"
      mv "${SOURCE_DIR}.tmp" "$SOURCE_DIR"
    fi
    ok "Source synced from local checkout → $SOURCE_DIR"
    return
  fi

  if [[ -d "$SOURCE_DIR/.git" ]]; then
    log "Updating existing clone ($NORIX_REF)…"
    git -C "$SOURCE_DIR" fetch --depth 1 origin "$NORIX_REF"
    git -C "$SOURCE_DIR" checkout -f FETCH_HEAD
  else
    log "Cloning $NORIX_REPO @$NORIX_REF → $SOURCE_DIR"
    rm -rf "$SOURCE_DIR"
    git clone --depth 1 --branch "$NORIX_REF" "$NORIX_REPO" "$SOURCE_DIR" 2>/dev/null \
      || git clone --depth 1 "$NORIX_REPO" "$SOURCE_DIR" && git -C "$SOURCE_DIR" checkout "$NORIX_REF"
  fi
  [[ -f "$SOURCE_DIR/$COMPOSE_FILE" ]] || die "compose file missing at $SOURCE_DIR/$COMPOSE_FILE (wrong NORIX_REF?)"
  ok "Source ready"
}

write_env() {
  local public_url app_pw owner_pw secrets_key
  public_url="$(detect_public_url)"

  if [[ -f "$ENV_FILE" ]]; then
    log "Keeping existing $ENV_FILE (upgrade-safe)"
    # Refresh public URL / port if still defaults
    grep -q '^NORIX_HTTP_PORT=' "$ENV_FILE" || echo "NORIX_HTTP_PORT=$NORIX_HTTP_PORT" >>"$ENV_FILE"
    return
  fi

  app_pw="$(openssl rand -hex 24)"
  owner_pw="$(openssl rand -hex 24)"
  secrets_key="$(openssl rand -hex 32)"

  cat >"$ENV_FILE" <<EOF
POSTGRES_DB=norix
POSTGRES_USER=norix
POSTGRES_PASSWORD=${owner_pw}
POSTGRES_APP_PASSWORD=${app_pw}
DATABASE_URL=postgres://norix_app:${app_pw}@postgres:5432/norix?sslmode=disable
MIGRATION_DATABASE_URL=postgres://norix:${owner_pw}@postgres:5432/norix?sslmode=disable
NORIX_VERSION=1.0.0-beta.1
NORIX_HTTP_PORT=${NORIX_HTTP_PORT}
NORIX_PUBLIC_URL=${public_url}
NORIX_WEB_ORIGIN=${public_url}
CORS_ALLOWED_ORIGINS=*
NORIX_REGISTRY=none
NORIX_PULL_POLICY=${NORIX_PULL_POLICY}
NORIX_BUILD_SECRETS_KEY=${secrets_key}
LOG_LEVEL=info
EOF
  chmod 600 "$ENV_FILE"
  ok "Wrote $ENV_FILE"
}

start_stack() {
  cd "$SOURCE_DIR"
  log "Building and starting Norix (this may take several minutes on first run)…"
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d --build
  ok "Compose is up"
}

wait_health() {
  local url i
  url="http://127.0.0.1:${NORIX_HTTP_PORT}/healthz"
  log "Waiting for control plane at $url …"
  for i in $(seq 1 60); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      ok "Healthy"
      return
    fi
    sleep 2
  done
  die "control plane did not become healthy — check: docker compose -f $SOURCE_DIR/$COMPOSE_FILE logs control-plane"
}

print_next() {
  local public_url
  public_url="$(grep -E '^NORIX_PUBLIC_URL=' "$ENV_FILE" | cut -d= -f2-)"
  cat <<EOF

$(printf '\033[32m✔\033[0m') Norix is installed (beta self-host).

  UI / API:   ${public_url}
  Health:     http://127.0.0.1:${NORIX_HTTP_PORT}/healthz
  Data:       ${NORIX_DATA}
  Compose:    ${SOURCE_DIR}/${COMPOSE_FILE}
  Env:        ${ENV_FILE}

Next:
  1. Open the UI and create the first admin (bootstrap).
  2. Deploy nginx:alpine from a project to verify the local docker runtime.
  3. Add more nodes with scripts/install-agent.sh (join token from Nodes).

Upgrade later:
  curl -fsSL ${NORIX_GITHUB_RAW}/${NORIX_REF}/scripts/get-norix.sh | NORIX_REF=${NORIX_REF} bash

Back up ${ENV_FILE} off-box (DB passwords + build secrets key).

EOF
}

# ── main ───────────────────────────────────────────────────────────────────
log "Norix self-host installer (ref=${NORIX_REF})"
install_docker
sync_source
command -v openssl >/dev/null 2>&1 || die "openssl is required to generate secrets"
write_env
# shellcheck disable=SC1090
set -a && source "$ENV_FILE" && set +a
NORIX_HTTP_PORT="${NORIX_HTTP_PORT:-8000}"
start_stack
wait_health
print_next
