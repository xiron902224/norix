#!/usr/bin/env bash
# Norix self-host installer — one-click for docker | bare.
#
# Until get.norix.dev exists, install from GitHub:
#
#   # Docker (default): Postgres + control plane via Compose
#   curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/v1.0.0-beta.5/scripts/get-norix.sh \
#     | NORIX_REF=v1.0.0-beta.5 bash
#
#   # Bare (VPS with systemd): system Postgres + binary + systemd
#   curl -fsSL …/get-norix.sh | NORIX_REF=v1.0.0-beta.5 bash -s -- --mode bare
#
#   # Bare without systemd (labs / containers): same stack, nohup supervisor
#   curl -fsSL …/get-norix.sh | bash -s -- --mode bare --supervise none
#
# Env:
#   NORIX_MODE            docker|bare (overridden by --mode)
#   NORIX_SUPERVISE       auto|systemd|none (overridden by --supervise; bare only)
#   NORIX_REF             git ref to clone (default: main)
#   NORIX_REPO            git URL
#   NORIX_DATA            data root (default: /data/norix)
#   NORIX_HTTP_PORT       panel port (default: 8000; also --port)
#   NORIX_PUBLIC_URL      public URL (also --public-url)
#   NORIX_PANEL_DOMAIN    enable panel Caddy HTTPS (also --panel-domain) (#26)
#   NORIX_ACME_EMAIL      ACME account email (also --acme-email)
#   NORIX_ACME_CA         optional ACME directory (Pebble/staging)
#   NORIX_SKIP_DOCKER     if 1, do not install Docker in docker mode
#   NORIX_PULL_POLICY     build|missing|always (docker mode)
#   NORIX_VERSION         release tag for bare binary (default: VERSION file / ref)
#   NORIX_MIN_RAM_MB      bare preflight soft warning threshold (default: 900)
#   NORIX_BINARY_FILE     local path to norix-control-plane binary (skip download)
#   NORIX_SKIP_CHECKSUM   if 1, skip SHA256 when using NORIX_BINARY_FILE only
#   NORIX_SKIP_COSIGN     if 1, skip cosign verify when a .cosign.bundle is present
#   NORIX_COSIGN_IDENTITY_REGEXP  override certificate identity regexp (keyless)

set -euo pipefail

NORIX_REPO="${NORIX_REPO:-https://github.com/xiron902224/norix.git}"
NORIX_REF="${NORIX_REF:-main}"
NORIX_DATA="${NORIX_DATA:-/data/norix}"
NORIX_HTTP_PORT="${NORIX_HTTP_PORT:-8000}"
NORIX_SKIP_DOCKER="${NORIX_SKIP_DOCKER:-0}"
NORIX_PULL_POLICY="${NORIX_PULL_POLICY:-build}"
NORIX_MODE="${NORIX_MODE:-docker}"
NORIX_SUPERVISE="${NORIX_SUPERVISE:-auto}"
NORIX_GITHUB_RAW="${NORIX_GITHUB_RAW:-https://raw.githubusercontent.com/xiron902224/norix}"
NORIX_GITHUB_REPO="${NORIX_GITHUB_REPO:-xiron902224/norix}"
NORIX_MIN_RAM_MB="${NORIX_MIN_RAM_MB:-900}"
NORIX_YES="${NORIX_YES:-0}"
NORIX_PANEL_DOMAIN="${NORIX_PANEL_DOMAIN:-}"
NORIX_ACME_EMAIL="${NORIX_ACME_EMAIL:-}"
NORIX_ACME_CA="${NORIX_ACME_CA:-}"
NORIX_SKIP_COSIGN="${NORIX_SKIP_COSIGN:-0}"
NORIX_COSIGN_OIDC_ISSUER="${NORIX_COSIGN_OIDC_ISSUER:-https://token.actions.githubusercontent.com}"
# Keyless Sigstore identity for artifacts signed by this repo's release.yml.
NORIX_COSIGN_IDENTITY_REGEXP="${NORIX_COSIGN_IDENTITY_REGEXP:-https://github.com/${NORIX_GITHUB_REPO}/\\.github/workflows/release\\.yml@.*}"

SOURCE_DIR="${NORIX_DATA}/source"
ENV_FILE="${SOURCE_DIR}/.env"
COMPOSE_FILE="deploy/norix/docker-compose.prod.yml"
BARE_ENV="/etc/norix/control-plane.env"
INSTALL_STATE="/var/lib/norix/install-state.json"
BARE_PID_FILE="/var/lib/norix/norix-control-plane.pid"
BARE_LOG_FILE="/var/lib/norix/norix-control-plane.log"
BARE_STEP=0
BARE_STEPS=8

log() { printf '\033[36m==>\033[0m %s\n' "$*"; }
ok() { printf '\033[32m✔\033[0m %s\n' "$*"; }
warn() { printf '\033[33mwarn:\033[0m %s\n' "$*" >&2; }
die() { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

step() {
  BARE_STEP=$((BARE_STEP + 1))
  printf '\033[36m[%s/%s]\033[0m %s\n' "$BARE_STEP" "$BARE_STEPS" "$*"
}

usage() {
  cat <<'EOF'
Usage: get-norix.sh [options]

Options:
  --mode docker|bare     Install mode (default: docker)
  --supervise auto|systemd|none
                         Bare process supervisor (default: auto).
                         auto → systemd when available, else fail with hint.
                         none → nohup (labs / containers without systemd)
  --port N               HTTP port (default: 8000)
  --public-url URL       Public UI/API URL
  --panel-domain HOST    Put Caddy in front of the panel with ACME HTTPS (#26)
  --acme-email EMAIL     ACME account email (required with --panel-domain)
  --yes                  Non-interactive (reserved; always noninteractive today)
  -h, --help             Show this help

Examples:
  curl -fsSL …/get-norix.sh | bash
  curl -fsSL …/get-norix.sh | bash -s -- --panel-domain norix.example.com --acme-email ops@example.com
  curl -fsSL …/get-norix.sh | NORIX_REF=v1.0.0-beta.5 bash -s -- --mode bare
  curl -fsSL …/get-norix.sh | bash -s -- --mode bare --supervise none --port 8000
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --mode)
      NORIX_MODE="${2:-}"
      [[ -n "$NORIX_MODE" ]] || die "--mode requires docker|bare"
      shift 2
      ;;
    --mode=*)
      NORIX_MODE="${1#--mode=}"
      shift
      ;;
    --supervise)
      NORIX_SUPERVISE="${2:-}"
      [[ -n "$NORIX_SUPERVISE" ]] || die "--supervise requires auto|systemd|none"
      shift 2
      ;;
    --supervise=*)
      NORIX_SUPERVISE="${1#--supervise=}"
      shift
      ;;
    --port)
      NORIX_HTTP_PORT="${2:-}"
      [[ -n "$NORIX_HTTP_PORT" ]] || die "--port requires a value"
      shift 2
      ;;
    --port=*)
      NORIX_HTTP_PORT="${1#--port=}"
      shift
      ;;
    --public-url)
      NORIX_PUBLIC_URL="${2:-}"
      [[ -n "${NORIX_PUBLIC_URL}" ]] || die "--public-url requires a value"
      shift 2
      ;;
    --public-url=*)
      NORIX_PUBLIC_URL="${1#--public-url=}"
      shift
      ;;
    --panel-domain)
      NORIX_PANEL_DOMAIN="${2:-}"
      [[ -n "${NORIX_PANEL_DOMAIN}" ]] || die "--panel-domain requires a hostname"
      shift 2
      ;;
    --panel-domain=*)
      NORIX_PANEL_DOMAIN="${1#--panel-domain=}"
      shift
      ;;
    --acme-email)
      NORIX_ACME_EMAIL="${2:-}"
      [[ -n "${NORIX_ACME_EMAIL}" ]] || die "--acme-email requires a value"
      shift 2
      ;;
    --acme-email=*)
      NORIX_ACME_EMAIL="${1#--acme-email=}"
      shift
      ;;
    --yes|-y)
      NORIX_YES=1
      shift
      ;;
    *)
      die "unknown argument: $1 (try --help)"
      ;;
  esac
done

if [[ -n "${NORIX_PANEL_DOMAIN}" && -z "${NORIX_ACME_EMAIL}" ]]; then
  die "--panel-domain requires --acme-email (ACME account)"
fi
if [[ -n "${NORIX_PANEL_DOMAIN}" && -z "${NORIX_PUBLIC_URL:-}" ]]; then
  NORIX_PUBLIC_URL="https://${NORIX_PANEL_DOMAIN}"
fi

case "$NORIX_MODE" in
  docker|bare) ;;
  *) die "invalid NORIX_MODE/ --mode: $NORIX_MODE (use docker|bare)" ;;
esac

case "$NORIX_SUPERVISE" in
  auto|systemd|none) ;;
  *) die "invalid --supervise / NORIX_SUPERVISE: $NORIX_SUPERVISE (use auto|systemd|none)" ;;
esac

if [[ "$(id -u)" -ne 0 ]]; then
  die "run as root (or: curl … | sudo bash)"
fi

ensure_minimal_tools() {
  if command -v curl >/dev/null 2>&1 && command -v openssl >/dev/null 2>&1; then
    return 0
  fi
  log "Installing curl / openssl…"
  if command -v apt-get >/dev/null 2>&1; then
    DEBIAN_FRONTEND=noninteractive apt-get update -y
    DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl openssl
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y ca-certificates curl openssl
  fi
  command -v curl >/dev/null 2>&1 || die "curl is required"
  command -v openssl >/dev/null 2>&1 || die "openssl is required to generate secrets"
}
ensure_minimal_tools

# ── helpers ────────────────────────────────────────────────────────────────

ensure_git() {
  command -v git >/dev/null 2>&1 && return
  log "Installing git…"
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -y && apt-get install -y git
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y git
  else
    die "git is required"
  fi
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *) die "unsupported architecture: $(uname -m)" ;;
  esac
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

resolve_version_tag() {
  local v="${NORIX_VERSION:-}"
  if [[ -z "$v" && -f "${SOURCE_DIR}/VERSION" ]]; then
    v="$(tr -d '[:space:]' <"${SOURCE_DIR}/VERSION")"
  fi
  if [[ -z "$v" ]]; then
    case "$NORIX_REF" in
      v*) v="${NORIX_REF}" ;;
      *) v="v${NORIX_REF}" ;;
    esac
  fi
  case "$v" in
    v*) echo "$v" ;;
    *) echo "v${v}" ;;
  esac
}

cgroup_memory_max_bytes() {
  if [[ -r /sys/fs/cgroup/memory.max ]]; then
    local v
    v="$(cat /sys/fs/cgroup/memory.max 2>/dev/null || true)"
    if [[ -n "$v" && "$v" != "max" ]]; then
      printf '%s' "$v"
      return
    fi
  fi
  if [[ -r /sys/fs/cgroup/memory/memory.limit_in_bytes ]]; then
    cat /sys/fs/cgroup/memory/memory.limit_in_bytes 2>/dev/null || true
  fi
}

has_working_systemd() {
  command -v systemctl >/dev/null 2>&1 || return 1
  # PID 1 should be systemd for a real host/VPS.
  local comm
  comm="$(cat /proc/1/comm 2>/dev/null || true)"
  [[ "$comm" == "systemd" ]] || return 1
  systemctl is-system-running >/dev/null 2>&1 || systemctl is-system-running 2>/dev/null | grep -Eq 'running|degraded'
}

resolve_supervise() {
  case "$NORIX_SUPERVISE" in
    systemd)
      has_working_systemd || die "systemd requested (--supervise systemd) but PID 1 is not systemd / systemctl is offline.
  On a real VPS this should work. In Docker/lab containers use: --supervise none"
      echo systemd
      ;;
    none)
      echo none
      ;;
    auto)
      if has_working_systemd; then
        echo systemd
      else
        die "bare mode needs systemd on a real VPS (PID 1=$(cat /proc/1/comm 2>/dev/null || echo unknown)).
  Fix: install on a normal Ubuntu/Debian VPS, OR re-run with:
    bash -s -- --mode bare --supervise none
  Docs: docs/runbooks/BARE_INSTALL.md"
      fi
      ;;
  esac
}

run_as_postgres() {
  # Avoid sudo dependency: prefer runuser, then su.
  if command -v runuser >/dev/null 2>&1; then
    runuser -u postgres -- "$@"
  elif command -v su >/dev/null 2>&1; then
    # su -c joins args into one shell string when using dash/bash.
    local cmd=""
    local a
    for a in "$@"; do
      cmd+="$(printf '%q ' "$a")"
    done
    su -s /bin/bash postgres -c "$cmd"
  else
    die "cannot run as postgres OS user (need runuser or su)"
  fi
}

psql_as_postgres() {
  run_as_postgres psql -v ON_ERROR_STOP=1 "$@"
}

write_install_state() {
  local supervise="$1"
  local tag
  tag="$(resolve_version_tag)"
  mkdir -p /var/lib/norix
  cat >"$INSTALL_STATE" <<EOF
{
  "mode": "${NORIX_MODE}",
  "supervise": "${supervise}",
  "ref": "${NORIX_REF}",
  "version": "${tag}",
  "http_port": ${NORIX_HTTP_PORT},
  "installed_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "data_dir": "${NORIX_DATA}"
}
EOF
  chmod 644 "$INSTALL_STATE"
}

# ── source sync ────────────────────────────────────────────────────────────

sync_source() {
  ensure_git
  mkdir -p "$NORIX_DATA"/{backups,agent}
  chmod 700 "$NORIX_DATA"

  local here
  here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." 2>/dev/null && pwd || true)"
  if [[ -n "$here" && -f "$here/scripts/get-norix.sh" && -f "$here/VERSION" ]]; then
    log "Using local repository at $here"
    mkdir -p "$SOURCE_DIR"
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
    ok "Source synced → $SOURCE_DIR"
    return
  fi

  if [[ -d "$SOURCE_DIR/.git" ]]; then
    log "Updating existing clone ($NORIX_REF)…"
    if git -C "$SOURCE_DIR" fetch --depth 1 origin "$NORIX_REF"; then
      git -C "$SOURCE_DIR" checkout -f FETCH_HEAD
    else
      warn "git fetch failed — keeping existing checkout at $SOURCE_DIR"
      [[ -f "$SOURCE_DIR/VERSION" ]] || die "clone missing VERSION and fetch failed"
    fi
  else
    log "Cloning $NORIX_REPO @$NORIX_REF → $SOURCE_DIR"
    rm -rf "$SOURCE_DIR"
    if ! git clone --depth 1 --branch "$NORIX_REF" "$NORIX_REPO" "$SOURCE_DIR" 2>/dev/null; then
      git clone --depth 1 "$NORIX_REPO" "$SOURCE_DIR" \
        || die "git clone failed (network / NORIX_REPO). Fix DNS or copy source to $SOURCE_DIR"
      git -C "$SOURCE_DIR" checkout "$NORIX_REF" \
        || die "checkout $NORIX_REF failed"
    fi
  fi
  [[ -f "$SOURCE_DIR/VERSION" ]] || die "clone missing VERSION (wrong NORIX_REF?)"
  ok "Source ready"
}

# Ensure NORIX_BUILD_SECRETS_KEY exists (CP fails closed without it unless
# NORIX_ALLOW_INSECURE_DEV_SECRETS=1).
ensure_build_secrets_key() {
  local env_file="$1"
  [[ -f "$env_file" ]] || return 0
  local existing
  existing="$(grep -E '^NORIX_BUILD_SECRETS_KEY=' "$env_file" 2>/dev/null | head -n1 | cut -d= -f2- || true)"
  if [[ -n "$existing" ]]; then
    return 0
  fi
  local secrets_key
  secrets_key="$(openssl rand -hex 32)"
  if grep -q '^NORIX_BUILD_SECRETS_KEY=' "$env_file" 2>/dev/null; then
    sed -i "s|^NORIX_BUILD_SECRETS_KEY=.*|NORIX_BUILD_SECRETS_KEY=${secrets_key}|" "$env_file"
  else
    echo "NORIX_BUILD_SECRETS_KEY=${secrets_key}" >>"$env_file"
  fi
  log "Generated NORIX_BUILD_SECRETS_KEY in $env_file"
}

# ── docker mode ────────────────────────────────────────────────────────────

install_docker() {
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    ok "Docker Engine + Compose already present"
    return
  fi
  [[ "$NORIX_SKIP_DOCKER" == "1" ]] && die "Docker/Compose missing and NORIX_SKIP_DOCKER=1"
  log "Installing Docker Engine…"
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker 2>/dev/null || true
  command -v docker >/dev/null 2>&1 || die "Docker install failed"
  docker compose version >/dev/null 2>&1 || die "Docker Compose v2 plugin missing"
  ok "Docker installed"
}

ensure_join_token_env() {
  local env_file="$1"
  local join_token
  if grep -qE '^NORIX_JOIN_TOKEN=.+' "$env_file" 2>/dev/null; then
    if ! grep -qE '^NORIX_BOOTSTRAP_JOIN_TOKEN=.+' "$env_file" 2>/dev/null; then
      join_token="$(grep -E '^NORIX_JOIN_TOKEN=' "$env_file" | head -n1 | cut -d= -f2-)"
      echo "NORIX_BOOTSTRAP_JOIN_TOKEN=${join_token}" >>"$env_file"
    fi
    return 0
  fi
  join_token="$(openssl rand -base64 32 | tr -d '=\n/' | head -c 43)"
  {
    echo "NORIX_JOIN_TOKEN=${join_token}"
    echo "NORIX_BOOTSTRAP_JOIN_TOKEN=${join_token}"
    echo "NORIX_NODE_NAME=${NORIX_NODE_NAME:-norix-1}"
  } >>"$env_file"
  ok "Wrote NORIX_JOIN_TOKEN for Compose agent (#10)"
}

warn_docker_socket_exposure() {
  cat >&2 <<'EOF'
WARNING (#10): the Compose agent mounts /var/run/docker.sock (≈ host root on rootful Docker).
  The control-plane container does NOT get the socket by default.
  Docs: https://github.com/xiron902224/norix/blob/main/docs/user/docker-socket.md
        https://github.com/xiron902224/norix/blob/main/docs/user/es/docker-socket.md
  Opt-in embedded CP runtime (not recommended): docker-compose.embedded-runtime.yml
EOF
}

write_env_docker() {
  local public_url app_pw owner_pw secrets_key
  public_url="$(detect_public_url)"

  if [[ -f "$ENV_FILE" ]]; then
    log "Keeping existing $ENV_FILE (upgrade-safe)"
    grep -q '^NORIX_HTTP_PORT=' "$ENV_FILE" || echo "NORIX_HTTP_PORT=$NORIX_HTTP_PORT" >>"$ENV_FILE"
    ensure_build_secrets_key "$ENV_FILE"
    ensure_join_token_env "$ENV_FILE"
    return
  fi

  app_pw="$(openssl rand -hex 24)"
  owner_pw="$(openssl rand -hex 24)"
  secrets_key="$(openssl rand -hex 32)"
  local join_token
  join_token="$(openssl rand -base64 32 | tr -d '=\n/' | head -c 43)"

  cat >"$ENV_FILE" <<EOF
POSTGRES_DB=norix
POSTGRES_USER=norix
POSTGRES_PASSWORD=${owner_pw}
POSTGRES_APP_PASSWORD=${app_pw}
DATABASE_URL=postgres://norix_app:${app_pw}@postgres:5432/norix?sslmode=disable
MIGRATION_DATABASE_URL=postgres://norix:${owner_pw}@postgres:5432/norix?sslmode=disable
NORIX_VERSION=${NORIX_VERSION:-$(tr -d '[:space:]' <"${SOURCE_DIR}/VERSION" 2>/dev/null || echo "${NORIX_REF#v}")}
NORIX_HTTP_PORT=${NORIX_HTTP_PORT}
NORIX_PUBLIC_URL=${public_url}
NORIX_WEB_ORIGIN=${public_url}
CORS_ALLOWED_ORIGINS=${public_url}
NORIX_REGISTRY=none
NORIX_PULL_POLICY=${NORIX_PULL_POLICY}
NORIX_BUILD_SECRETS_KEY=${secrets_key}
NORIX_JOIN_TOKEN=${join_token}
NORIX_BOOTSTRAP_JOIN_TOKEN=${join_token}
NORIX_NODE_NAME=norix-1
LOG_LEVEL=info
NORIX_INSTALL_MODE=docker
# Docker published ports rarely look like loopback inside the container — allow
# first-admin bootstrap over HTTP until you put TLS in front, then remove this.
NORIX_ALLOW_INSECURE_BOOTSTRAP=1
EOF
  if [[ -n "${NORIX_PANEL_DOMAIN}" ]]; then
    cat >>"$ENV_FILE" <<EOF
NORIX_PANEL_DOMAIN=${NORIX_PANEL_DOMAIN}
NORIX_ACME_EMAIL=${NORIX_ACME_EMAIL}
NORIX_ACME_CA=${NORIX_ACME_CA}
NORIX_PUBLIC_URL=https://${NORIX_PANEL_DOMAIN}
NORIX_WEB_ORIGIN=https://${NORIX_PANEL_DOMAIN}
CORS_ALLOWED_ORIGINS=https://${NORIX_PANEL_DOMAIN}
# Panel Caddy terminates TLS — prefer secure bootstrap.
NORIX_ALLOW_INSECURE_BOOTSTRAP=0
EOF
  fi
  chmod 600 "$ENV_FILE"
  ok "Wrote $ENV_FILE"
}

start_stack_docker() {
  [[ -f "$SOURCE_DIR/$COMPOSE_FILE" ]] || die "missing $SOURCE_DIR/$COMPOSE_FILE"
  cd "$SOURCE_DIR"
  log "Building and starting Norix (first run may take several minutes)…"
  local compose_args=(-f "$COMPOSE_FILE")
  if [[ -n "${NORIX_PANEL_DOMAIN}" ]] || grep -q '^NORIX_PANEL_DOMAIN=.\+' "$ENV_FILE" 2>/dev/null; then
    [[ -f deploy/norix/docker-compose.panel-tls.yml ]] || die "missing panel-tls compose overlay"
    compose_args+=(-f deploy/norix/docker-compose.panel-tls.yml)
    log "Panel TLS enabled (Caddy + ACME for ${NORIX_PANEL_DOMAIN:-$(grep NORIX_PANEL_DOMAIN "$ENV_FILE" | cut -d= -f2-)})"
  fi
  docker compose --env-file "$ENV_FILE" "${compose_args[@]}" up -d --build
  ok "Compose is up"
}

# ── bare mode ──────────────────────────────────────────────────────────────

preflight_bare() {
  step "Preflight"
  detect_arch >/dev/null
  command -v curl >/dev/null 2>&1 || die "curl is required"
  command -v openssl >/dev/null 2>&1 || die "openssl is required"

  local mem_bytes mem_mb
  mem_bytes="$(cgroup_memory_max_bytes || true)"
  if [[ -n "${mem_bytes:-}" && "$mem_bytes" =~ ^[0-9]+$ ]]; then
    mem_mb=$((mem_bytes / 1024 / 1024))
    if (( mem_mb < NORIX_MIN_RAM_MB )); then
      warn "cgroup memory limit is ${mem_mb}MiB (< ${NORIX_MIN_RAM_MB}MiB recommended); install may OOM"
    fi
  fi

  if ss -lnt 2>/dev/null | awk '{print $4}' | grep -Eq "[:.]${NORIX_HTTP_PORT}\$"; then
    # Allow re-run if our own process already holds the port.
    if [[ -f "$BARE_PID_FILE" ]] && kill -0 "$(cat "$BARE_PID_FILE")" 2>/dev/null; then
      ok "Port ${NORIX_HTTP_PORT} already served by existing Norix PID $(cat "$BARE_PID_FILE")"
    elif has_working_systemd && systemctl is-active --quiet norix-control-plane 2>/dev/null; then
      ok "Port ${NORIX_HTTP_PORT} already served by norix-control-plane.service"
    else
      die "port ${NORIX_HTTP_PORT} is already in use — free it or pass --port"
    fi
  fi

  SUPERVISE_MODE="$(resolve_supervise)"
  ok "Preflight OK (arch=$(detect_arch), supervise=${SUPERVISE_MODE}, port=${NORIX_HTTP_PORT})"
}

ensure_pkg_ca_curl() {
  if command -v apt-get >/dev/null 2>&1; then
    DEBIAN_FRONTEND=noninteractive apt-get update -y
    DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl openssl
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y ca-certificates curl openssl
  fi
}

start_postgres_service() {
  if pg_isready -h 127.0.0.1 >/dev/null 2>&1; then
    return 0
  fi

  if [[ "${SUPERVISE_MODE}" == "systemd" ]]; then
    systemctl enable --now postgresql 2>/dev/null \
      || systemctl enable --now postgresql.service 2>/dev/null \
      || true
  fi

  # Cluster helpers work with or without systemd (Ubuntu/Debian).
  if command -v pg_ctlcluster >/dev/null 2>&1; then
    local ver cluster
    while read -r ver cluster _; do
      [[ -n "$ver" && -n "$cluster" ]] || continue
      pg_ctlcluster "$ver" "$cluster" start 2>/dev/null || true
    done < <(pg_lsclusters -h 2>/dev/null || true)
  fi

  if command -v pg_ctl >/dev/null 2>&1 && id postgres >/dev/null 2>&1; then
    # Last resort: try common data dirs.
    local d
    for d in /var/lib/postgresql/*/main; do
      [[ -d "$d" ]] || continue
      run_as_postgres pg_ctl -D "$d" -w start 2>/dev/null || true
    done
  fi

  local i
  for i in $(seq 1 30); do
    pg_isready -h 127.0.0.1 >/dev/null 2>&1 && return 0
    sleep 1
  done
  return 1
}

install_postgres_bare() {
  step "PostgreSQL"
  if command -v psql >/dev/null 2>&1 && pg_isready -h 127.0.0.1 >/dev/null 2>&1; then
    ok "PostgreSQL already reachable on 127.0.0.1"
    return
  fi

  if ! command -v psql >/dev/null 2>&1; then
    log "Installing PostgreSQL…"
    if command -v apt-get >/dev/null 2>&1; then
      DEBIAN_FRONTEND=noninteractive apt-get update -y
      DEBIAN_FRONTEND=noninteractive apt-get install -y postgresql postgresql-contrib
    elif command -v dnf >/dev/null 2>&1; then
      dnf install -y postgresql-server postgresql
      postgresql-setup --initdb 2>/dev/null || true
    else
      die "cannot install PostgreSQL automatically — set up Postgres and re-run, or use --mode docker"
    fi
  fi

  start_postgres_service || die "PostgreSQL installed but not ready (pg_isready failed).
  Check: pg_lsclusters; journalctl -u postgresql -e (systemd hosts)"
  ok "PostgreSQL ready"
}

ensure_norix_db_bare() {
  local owner_pw="$1" app_pw="$2"
  local db_name=norix db_owner=norix db_app=norix_app

  id postgres >/dev/null 2>&1 || die "OS user 'postgres' not found after PostgreSQL install"

  psql_as_postgres <<SQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${db_owner}') THEN
    CREATE ROLE ${db_owner} LOGIN PASSWORD '${owner_pw}' SUPERUSER;
  ELSE
    ALTER ROLE ${db_owner} WITH LOGIN PASSWORD '${owner_pw}' SUPERUSER;
  END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${db_app}') THEN
    CREATE ROLE ${db_app} LOGIN PASSWORD '${app_pw}' NOINHERIT;
  ELSE
    ALTER ROLE ${db_app} WITH LOGIN PASSWORD '${app_pw}' NOINHERIT;
  END IF;
END
\$\$;
SQL

  if ! run_as_postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${db_name}'" | grep -q 1; then
    run_as_postgres createdb -O "${db_owner}" "${db_name}"
  fi

  # App role needs connect + schema rights for RLS runtime.
  psql_as_postgres -d "$db_name" <<SQL
GRANT CONNECT ON DATABASE ${db_name} TO ${db_app};
GRANT USAGE, CREATE ON SCHEMA public TO ${db_app};
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ${db_app};
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO ${db_app};
SQL
  ok "Database ${db_name} and roles ready"
}

curl_retry() {
  # Retries help flaky GitHub Releases / DNS in labs.
  curl -fsSL --connect-timeout 15 --max-time 300 --retry 5 --retry-delay 2 --retry-all-errors "$@"
}

# Verify a Sigstore cosign bundle when present (fail-closed). SHA256 is checked separately.
verify_cosign_bundle() {
  local binary_path="$1" asset_name="$2" bundle_url="$3" bundle_path="$4"
  if [[ "${NORIX_SKIP_COSIGN}" == "1" ]]; then
    warn "Skipping cosign verification (NORIX_SKIP_COSIGN=1)"
    return 0
  fi

  local fetched=0
  if [[ -n "$bundle_path" && -f "$bundle_path" && -s "$bundle_path" ]]; then
    fetched=1
  elif [[ -n "$bundle_url" ]]; then
    if curl_retry -o "${bundle_path}" "$bundle_url" 2>/dev/null; then
      fetched=1
    else
      rm -f "${bundle_path}"
      warn "No cosign bundle for ${asset_name} — continuing with SHA256 only"
      return 0
    fi
  else
    warn "No cosign bundle URL for ${asset_name} — continuing with SHA256 only"
    return 0
  fi

  [[ "$fetched" -eq 1 ]] || return 0
  command -v cosign >/dev/null 2>&1 || die "cosign bundle present for ${asset_name} but cosign is not installed.
  Install: https://docs.sigstore.dev/cosign/system_config/installation/
  Or re-run with NORIX_SKIP_COSIGN=1 (labs only; not for production)."

  log "Verifying cosign bundle for ${asset_name}"
  if ! cosign verify-blob \
    --bundle "$bundle_path" \
    --certificate-identity-regexp "${NORIX_COSIGN_IDENTITY_REGEXP}" \
    --certificate-oidc-issuer "${NORIX_COSIGN_OIDC_ISSUER}" \
    "$binary_path"; then
    die "cosign verification failed for ${asset_name} — refusing to install"
  fi
  ok "Cosign OK (${asset_name})"
}

install_control_plane_binary_bare() {
  step "Control-plane binary"
  local arch tag asset url sums_url bundle_url tmpdir dest expected actual src
  arch="$(detect_arch)"
  tag="$(resolve_version_tag)"
  asset="norix-control-plane-linux-${arch}"
  dest="/usr/local/bin/norix-control-plane"
  url="https://github.com/${NORIX_GITHUB_REPO}/releases/download/${tag}/${asset}"
  sums_url="https://github.com/${NORIX_GITHUB_REPO}/releases/download/${tag}/SHA256SUMS"
  bundle_url="https://github.com/${NORIX_GITHUB_REPO}/releases/download/${tag}/${asset}.cosign.bundle"

  tmpdir="$(mktemp -d)"
  # shellcheck disable=SC2064
  trap "rm -rf '$tmpdir'" RETURN

  if [[ -n "${NORIX_BINARY_FILE:-}" ]]; then
    [[ -f "$NORIX_BINARY_FILE" && -s "$NORIX_BINARY_FILE" ]] \
      || die "NORIX_BINARY_FILE not found or empty: $NORIX_BINARY_FILE"
    src="$NORIX_BINARY_FILE"
    log "Using local binary $src"
    cp -f "$src" "${tmpdir}/${asset}"
  else
    log "Downloading ${url}"
    if ! curl_retry -o "${tmpdir}/${asset}" "$url"; then
      die "release asset missing or download failed for ${tag} (${asset}).
  Publish the GitHub Release assets, pin NORIX_VERSION/NORIX_REF, or set NORIX_BINARY_FILE=/path/to/binary.
  Build-from-source is not part of the one-click path."
    fi
  fi

  if [[ "${NORIX_SKIP_CHECKSUM:-0}" == "1" && -n "${NORIX_BINARY_FILE:-}" ]]; then
    warn "Skipping SHA256 verification (NORIX_SKIP_CHECKSUM=1 with NORIX_BINARY_FILE)"
  else
    log "Verifying SHA256SUMS"
    if ! curl_retry -o "${tmpdir}/SHA256SUMS" "$sums_url"; then
      if [[ -n "${NORIX_BINARY_FILE:-}" ]]; then
        die "SHA256SUMS download failed. Re-run with NORIX_SKIP_CHECKSUM=1 for offline local binaries only."
      fi
      die "SHA256SUMS missing for ${tag} — refusing to install unverified binary"
    fi
    expected="$(awk -v f="$asset" '$2 == f {print $1; exit}' "${tmpdir}/SHA256SUMS")"
    [[ -n "$expected" ]] || die "asset ${asset} not listed in SHA256SUMS for ${tag}"
    actual="$(sha256sum "${tmpdir}/${asset}" | awk '{print $1}')"
    if [[ "$expected" != "$actual" ]]; then
      die "checksum mismatch for ${asset}
  expected: ${expected}
  actual:   ${actual}"
    fi
    ok "Checksum OK (${actual:0:12}…)"
  fi

  # Cosign when the release published a bundle (fail-closed if present + cosign fails).
  if [[ "${NORIX_SKIP_CHECKSUM:-0}" == "1" && -n "${NORIX_BINARY_FILE:-}" ]]; then
    :
  else
    verify_cosign_bundle \
      "${tmpdir}/${asset}" \
      "$asset" \
      "$bundle_url" \
      "${tmpdir}/${asset}.cosign.bundle"
  fi

  install -m 755 "${tmpdir}/${asset}" "$dest"
  ok "Installed $dest (${tag})"
}

write_env_bare() {
  step "Config /etc/norix"
  local public_url app_pw owner_pw secrets_key
  public_url="$(detect_public_url)"
  mkdir -p /etc/norix /var/lib/norix/backups
  useradd --system --home /var/lib/norix --shell /usr/sbin/nologin norix 2>/dev/null || true
  chown -R norix:norix /var/lib/norix

  if [[ -f "$BARE_ENV" ]]; then
    log "Keeping existing $BARE_ENV (upgrade-safe)"
    # Refresh public URL / port if caller overrode them this run.
    if [[ -n "${NORIX_PUBLIC_URL:-}" ]]; then
      sed -i "s|^NORIX_PUBLIC_URL=.*|NORIX_PUBLIC_URL=${NORIX_PUBLIC_URL}|" "$BARE_ENV" || true
      sed -i "s|^NORIX_WEB_ORIGIN=.*|NORIX_WEB_ORIGIN=${NORIX_PUBLIC_URL}|" "$BARE_ENV" || true
    fi
    if grep -q '^HTTP_ADDR=' "$BARE_ENV"; then
      sed -i "s|^HTTP_ADDR=.*|HTTP_ADDR=0.0.0.0:${NORIX_HTTP_PORT}|" "$BARE_ENV" || true
    fi
    ensure_build_secrets_key "$BARE_ENV"
    mkdir -p "$SOURCE_DIR"
    cp -a "$BARE_ENV" "$ENV_FILE" 2>/dev/null || true
    # Ensure DB roles still match passwords in env on resume (no-op if already OK).
    app_pw="$(grep -E '^DATABASE_URL=' "$BARE_ENV" | sed -n 's|.*://norix_app:\([^@]*\)@.*|\1|p')"
    owner_pw="$(grep -E '^MIGRATION_DATABASE_URL=' "$BARE_ENV" | sed -n 's|.*://norix:\([^@]*\)@.*|\1|p')"
    if [[ -n "$app_pw" && -n "$owner_pw" ]]; then
      ensure_norix_db_bare "$owner_pw" "$app_pw"
    fi
    ok "Config ready (existing)"
    return
  fi

  app_pw="$(openssl rand -hex 24)"
  owner_pw="$(openssl rand -hex 24)"
  secrets_key="$(openssl rand -hex 32)"

  ensure_norix_db_bare "$owner_pw" "$app_pw"

  cat >"$BARE_ENV" <<EOF
DATABASE_URL=postgres://norix_app:${app_pw}@127.0.0.1:5432/norix?sslmode=disable
MIGRATION_DATABASE_URL=postgres://norix:${owner_pw}@127.0.0.1:5432/norix?sslmode=disable
HTTP_ADDR=0.0.0.0:${NORIX_HTTP_PORT}
LOG_LEVEL=info
CORS_ALLOWED_ORIGINS=${public_url}
NORIX_PUBLIC_URL=${public_url}
NORIX_WEB_ORIGIN=${public_url}
NORIX_BACKUP_DIR=/var/lib/norix/backups
NORIX_BUILD_SECRETS_KEY=${secrets_key}
NORIX_REGISTRY=none
NORIX_INSTALL_MODE=bare
NORIX_SUPERVISE=${SUPERVISE_MODE}
NORIX_VERSION=${NORIX_VERSION:-$(tr -d '[:space:]' <"${SOURCE_DIR}/VERSION" 2>/dev/null || echo "${NORIX_REF#v}")}
EOF
  chmod 600 "$BARE_ENV"
  chown norix:norix "$BARE_ENV"
  mkdir -p "$SOURCE_DIR"
  cp -a "$BARE_ENV" "$ENV_FILE"
  chmod 600 "$ENV_FILE"
  ok "Wrote $BARE_ENV"
}

stop_bare_daemon_if_running() {
  if [[ -f "$BARE_PID_FILE" ]]; then
    local old
    old="$(cat "$BARE_PID_FILE" 2>/dev/null || true)"
    if [[ -n "$old" ]] && kill -0 "$old" 2>/dev/null; then
      log "Stopping previous norix-control-plane PID $old"
      kill "$old" 2>/dev/null || true
      sleep 1
      kill -9 "$old" 2>/dev/null || true
    fi
    rm -f "$BARE_PID_FILE"
  fi
}

start_control_plane_systemd() {
  local unit_src="${SOURCE_DIR}/deploy/systemd/norix-control-plane.bare.service"
  [[ -f "$unit_src" ]] || unit_src="${SOURCE_DIR}/deploy/systemd/norix-control-plane.service"
  [[ -f "$unit_src" ]] || die "missing systemd unit under deploy/systemd/"
  cp "$unit_src" /etc/systemd/system/norix-control-plane.service
  systemctl daemon-reload
  systemctl enable --now norix-control-plane
  ok "systemd unit norix-control-plane enabled"
}

start_control_plane_none() {
  stop_bare_daemon_if_running
  [[ -x /usr/local/bin/norix-control-plane ]] || die "missing /usr/local/bin/norix-control-plane"
  [[ -f "$BARE_ENV" ]] || die "missing $BARE_ENV"

  # Ensure norix can read env + write logs.
  chown norix:norix "$BARE_ENV" /var/lib/norix
  touch "$BARE_LOG_FILE"
  chown norix:norix "$BARE_LOG_FILE"

  log "Starting norix-control-plane under nohup (--supervise none)"
  # runuser keeps correct uid; env file loaded in a login-free bash.
  runuser -u norix -- bash -c "
    set -a
    # shellcheck disable=SC1090
    . '${BARE_ENV}'
    set +a
    nohup /usr/local/bin/norix-control-plane >>'${BARE_LOG_FILE}' 2>&1 &
    echo \$! >'${BARE_PID_FILE}'
  " || {
    # Fallback if runuser missing: su
    su -s /bin/bash norix -c "
      set -a
      . '${BARE_ENV}'
      set +a
      nohup /usr/local/bin/norix-control-plane >>'${BARE_LOG_FILE}' 2>&1 &
      echo \$! >'${BARE_PID_FILE}'
    "
  }
  chmod 644 "$BARE_PID_FILE" 2>/dev/null || true
  sleep 2
  local pid stat
  pid="$(cat "$BARE_PID_FILE" 2>/dev/null || true)"
  [[ -n "$pid" ]] || die "norix-control-plane failed to start (no PID) — see ${BARE_LOG_FILE}"
  stat="$(ps -o stat= -p "$pid" 2>/dev/null | tr -d ' ' || true)"
  if [[ -z "$stat" || "$stat" == *Z* ]] || ! kill -0 "$pid" 2>/dev/null; then
    warn "Last log lines (${BARE_LOG_FILE}):"
    tail -n 40 "$BARE_LOG_FILE" 2>/dev/null || true
    die "norix-control-plane exited immediately — see ${BARE_LOG_FILE}"
  fi
  ok "norix-control-plane running (PID ${pid}, log ${BARE_LOG_FILE})"
}

start_control_plane_bare() {
  step "Start control plane (${SUPERVISE_MODE})"
  case "$SUPERVISE_MODE" in
    systemd) start_control_plane_systemd ;;
    none) start_control_plane_none ;;
    *) die "internal: unknown SUPERVISE_MODE=$SUPERVISE_MODE" ;;
  esac
}

# ── shared ─────────────────────────────────────────────────────────────────

wait_health() {
  local url i
  url="http://127.0.0.1:${NORIX_HTTP_PORT}/healthz"
  if [[ "$NORIX_MODE" == "bare" ]]; then
    step "Health check"
  else
    log "Waiting for control plane at $url …"
  fi
  for i in $(seq 1 60); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      ok "Healthy (${url})"
      return
    fi
    sleep 2
  done
  if [[ "$NORIX_MODE" == "bare" ]]; then
    if [[ "${SUPERVISE_MODE:-}" == "none" ]]; then
      warn "Last log lines (${BARE_LOG_FILE}):"
      tail -n 40 "$BARE_LOG_FILE" 2>/dev/null || true
      die "not healthy — see ${BARE_LOG_FILE}"
    fi
    warn "Recent journal:"
    journalctl -u norix-control-plane -n 40 --no-pager 2>/dev/null || true
    die "not healthy — check: journalctl -u norix-control-plane -e"
  fi
  die "not healthy — check: docker compose -f $SOURCE_DIR/$COMPOSE_FILE logs control-plane"
}

print_next() {
  local public_url mode_note supervise_note=""
  if [[ -f "$ENV_FILE" ]]; then
    public_url="$(grep -E '^NORIX_PUBLIC_URL=' "$ENV_FILE" | cut -d= -f2-)"
  elif [[ -f "$BARE_ENV" ]]; then
    public_url="$(grep -E '^NORIX_PUBLIC_URL=' "$BARE_ENV" | cut -d= -f2-)"
  else
    public_url="$(detect_public_url)"
  fi

  if [[ "$NORIX_MODE" == "bare" ]]; then
    mode_note="Mode: bare (no Docker for the control plane)."
    if [[ "${SUPERVISE_MODE:-}" == "none" ]]; then
      supervise_note="
  Supervise: none (nohup). Logs: ${BARE_LOG_FILE}
  PID file:  ${BARE_PID_FILE}
  Stop:      kill \$(cat ${BARE_PID_FILE})"
    else
      supervise_note="
  Supervise: systemd (norix-control-plane.service)
  Logs:      journalctl -u norix-control-plane -f"
    fi
    mode_note="${mode_note}
  To run container workloads on this host later, install Docker and the agent,
  or add worker nodes — see docs/runbooks/BARE_INSTALL.md.${supervise_note}"
  else
    mode_note="Mode: docker (Compose stack under ${SOURCE_DIR}).
  Agent holds docker.sock (control plane does not) — docs/user/docker-socket.md
  After first admin, the Compose agent enrolls via NORIX_BOOTSTRAP_JOIN_TOKEN."
  fi

  cat <<EOF

$(printf '\033[32m✔\033[0m') Norix is ready.

  UI / API:   ${public_url}
  Health:     http://127.0.0.1:${NORIX_HTTP_PORT}/healthz
  Data:       ${NORIX_DATA}
  ${mode_note}

Next (first admin / bootstrap):
  Bootstrap only accepts HTTPS or a loopback client (unless NORIX_ALLOW_INSECURE_BOOTSTRAP=1).

  Preferred — domain + TLS in front of the panel:
    docs/user/production-tls.md  (EN) · docs/user/es/production-tls.md (ES)

  Or SSH tunnel, then open the local URL:
    ssh -L ${NORIX_HTTP_PORT}:127.0.0.1:${NORIX_HTTP_PORT} USER@HOST
    open http://127.0.0.1:${NORIX_HTTP_PORT}

  Docker one-click writes NORIX_ALLOW_INSECURE_BOOTSTRAP=1 for published ports;
  remove it after the first admin exists and TLS is configured.

  Then: create the first admin → wait for agent node online → deploy a workload.
  Extra workers: scripts/install-agent.sh (warns about docker.sock).

Upgrade / re-run (idempotent):
  curl -fsSL ${NORIX_GITHUB_RAW}/${NORIX_REF}/scripts/get-norix.sh \\
    | NORIX_REF=${NORIX_REF} bash -s -- --mode ${NORIX_MODE}${SUPERVISE_MODE:+ --supervise ${SUPERVISE_MODE}}

Back up env secrets off-box (${ENV_FILE} and/or ${BARE_ENV}).

EOF
}

run_bare() {
  BARE_STEP=0
  preflight_bare
  ensure_pkg_ca_curl
  step "Fetch source"
  sync_source
  install_postgres_bare
  write_env_bare
  # shellcheck disable=SC1090
  set -a && source "$BARE_ENV" && set +a
  # Prefer CLI/env port over stale HTTP_ADDR if both differ — write_env already synced.
  if [[ -n "${HTTP_ADDR:-}" && "$HTTP_ADDR" =~ :([0-9]+)$ ]]; then
    NORIX_HTTP_PORT="${NORIX_HTTP_PORT:-${BASH_REMATCH[1]}}"
  fi
  NORIX_HTTP_PORT="${NORIX_HTTP_PORT:-8000}"
  install_control_plane_binary_bare
  start_control_plane_bare
  wait_health
  write_install_state "$SUPERVISE_MODE"
  step "Done"
  print_next
}

# ── main ───────────────────────────────────────────────────────────────────

log "Norix installer (mode=${NORIX_MODE}, ref=${NORIX_REF})"

if [[ "$NORIX_MODE" == "docker" ]]; then
  sync_source
  install_docker
  write_env_docker
  # shellcheck disable=SC1090
  set -a && source "$ENV_FILE" && set +a
  NORIX_HTTP_PORT="${NORIX_HTTP_PORT:-8000}"
  warn_docker_socket_exposure
  start_stack_docker
  wait_health
  write_install_state "compose"
  print_next
else
  run_bare
fi
