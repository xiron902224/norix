#!/usr/bin/env bash
# Norix self-host installer.
#
# Until get.norix.dev exists, install from the GitHub repo:
#
#   # Docker mode (default): Postgres + control plane via Compose
#   curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/develop/scripts/get-norix.sh | bash
#
#   # Bare mode: system Postgres + control-plane binary + systemd (no Docker)
#   curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/develop/scripts/get-norix.sh \
#     | bash -s -- --mode bare
#
# Pin a branch / tag:
#   curl -fsSL …/get-norix.sh | NORIX_REF=v1.0.0-beta.1 bash -s -- --mode bare
#
# Env:
#   NORIX_MODE         docker|bare (default: docker; overridden by --mode)
#   NORIX_REF          git ref to clone (default: develop)
#   NORIX_REPO         git URL
#   NORIX_DATA         data root (default: /data/norix)
#   NORIX_HTTP_PORT    panel port (default: 8000)
#   NORIX_PUBLIC_URL   public URL (auto-detected if empty)
#   NORIX_SKIP_DOCKER  if 1, do not install Docker in docker mode
#   NORIX_PULL_POLICY  build|missing|always (docker mode)
#   NORIX_VERSION      release tag for binary download in bare mode (default: from VERSION file / ref)

set -euo pipefail

NORIX_REPO="${NORIX_REPO:-https://github.com/xiron902224/norix.git}"
NORIX_REF="${NORIX_REF:-develop}"
NORIX_DATA="${NORIX_DATA:-/data/norix}"
NORIX_HTTP_PORT="${NORIX_HTTP_PORT:-8000}"
NORIX_SKIP_DOCKER="${NORIX_SKIP_DOCKER:-0}"
NORIX_PULL_POLICY="${NORIX_PULL_POLICY:-build}"
NORIX_MODE="${NORIX_MODE:-docker}"
NORIX_GITHUB_RAW="${NORIX_GITHUB_RAW:-https://raw.githubusercontent.com/xiron902224/norix}"
NORIX_GITHUB_REPO="${NORIX_GITHUB_REPO:-xiron902224/norix}"

SOURCE_DIR="${NORIX_DATA}/source"
ENV_FILE="${SOURCE_DIR}/.env"
COMPOSE_FILE="deploy/norix/docker-compose.prod.yml"
BARE_ENV="/etc/norix/control-plane.env"

log() { printf '\033[36m==>\033[0m %s\n' "$*"; }
ok() { printf '\033[32m✔\033[0m %s\n' "$*"; }
die() { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Usage: get-norix.sh [--mode docker|bare] [--help]

  --mode docker   Postgres + control plane with Docker Compose (default)
  --mode bare     System Postgres + norix-control-plane binary + systemd (no Docker)

Examples:
  curl -fsSL …/get-norix.sh | bash
  curl -fsSL …/get-norix.sh | bash -s -- --mode bare
  NORIX_REF=v1.0.0-beta.1 curl -fsSL …/get-norix.sh | bash -s -- --mode bare
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
    *)
      die "unknown argument: $1 (try --help)"
      ;;
  esac
done

case "$NORIX_MODE" in
  docker|bare) ;;
  *) die "invalid NORIX_MODE/ --mode: $NORIX_MODE (use docker|bare)" ;;
esac

if [[ "$(id -u)" -ne 0 ]]; then
  die "run as root (or: curl … | sudo bash)"
fi

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v openssl >/dev/null 2>&1 || die "openssl is required to generate secrets"

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
    git -C "$SOURCE_DIR" fetch --depth 1 origin "$NORIX_REF"
    git -C "$SOURCE_DIR" checkout -f FETCH_HEAD
  else
    log "Cloning $NORIX_REPO @$NORIX_REF → $SOURCE_DIR"
    rm -rf "$SOURCE_DIR"
    git clone --depth 1 --branch "$NORIX_REF" "$NORIX_REPO" "$SOURCE_DIR" 2>/dev/null \
      || { git clone --depth 1 "$NORIX_REPO" "$SOURCE_DIR" && git -C "$SOURCE_DIR" checkout "$NORIX_REF"; }
  fi
  [[ -f "$SOURCE_DIR/VERSION" ]] || die "clone missing VERSION (wrong NORIX_REF?)"
  ok "Source ready"
}

# ── docker mode ────────────────────────────────────────────────────────────

write_env_docker() {
  local public_url app_pw owner_pw secrets_key
  public_url="$(detect_public_url)"

  if [[ -f "$ENV_FILE" ]]; then
    log "Keeping existing $ENV_FILE (upgrade-safe)"
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
NORIX_VERSION=1.0.0-beta.2
NORIX_HTTP_PORT=${NORIX_HTTP_PORT}
NORIX_PUBLIC_URL=${public_url}
NORIX_WEB_ORIGIN=${public_url}
CORS_ALLOWED_ORIGINS=*
NORIX_REGISTRY=none
NORIX_PULL_POLICY=${NORIX_PULL_POLICY}
NORIX_BUILD_SECRETS_KEY=${secrets_key}
LOG_LEVEL=info
NORIX_INSTALL_MODE=docker
EOF
  chmod 600 "$ENV_FILE"
  ok "Wrote $ENV_FILE"
}

start_stack_docker() {
  [[ -f "$SOURCE_DIR/$COMPOSE_FILE" ]] || die "missing $SOURCE_DIR/$COMPOSE_FILE"
  cd "$SOURCE_DIR"
  log "Building and starting Norix (first run may take several minutes)…"
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d --build
  ok "Compose is up"
}

# ── bare mode ──────────────────────────────────────────────────────────────

install_postgres_bare() {
  if command -v psql >/dev/null 2>&1 && systemctl is-active --quiet postgresql 2>/dev/null; then
    ok "PostgreSQL already running"
    return
  fi
  if command -v psql >/dev/null 2>&1 && pg_isready -h 127.0.0.1 >/dev/null 2>&1; then
    ok "PostgreSQL already reachable on 127.0.0.1"
    return
  fi
  log "Installing PostgreSQL…"
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -y
    apt-get install -y postgresql postgresql-contrib
    systemctl enable --now postgresql
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y postgresql-server postgresql
    postgresql-setup --initdb 2>/dev/null || true
    systemctl enable --now postgresql
  else
    die "cannot install PostgreSQL automatically — set up Postgres and re-run with existing DB, or use --mode docker"
  fi
  sleep 2
  pg_isready -h 127.0.0.1 >/dev/null 2>&1 || die "PostgreSQL installed but not ready"
  ok "PostgreSQL ready"
}

ensure_norix_db_bare() {
  local owner_pw="$1" app_pw="$2"
  local db_name=norix db_owner=norix db_app=norix_app

  # Prefer peer auth as postgres OS user.
  if id postgres >/dev/null 2>&1; then
    sudo -u postgres psql -v ON_ERROR_STOP=1 <<SQL
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
SELECT 'ok' FROM pg_database WHERE datname = '${db_name}';
SQL
    if ! sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${db_name}'" | grep -q 1; then
      sudo -u postgres createdb -O "${db_owner}" "${db_name}"
    fi
  else
    die "OS user 'postgres' not found — create DB roles manually (see docs/runbooks/BARE_INSTALL.md)"
  fi
  ok "Database ${db_name} and roles ready"
}

install_control_plane_binary_bare() {
  local arch tag asset url tmp dest
  arch="$(detect_arch)"
  tag="$(resolve_version_tag)"
  asset="norix-control-plane-linux-${arch}"
  dest="/usr/local/bin/norix-control-plane"
  tmp="$(mktemp)"

  url="https://github.com/${NORIX_GITHUB_REPO}/releases/download/${tag}/${asset}"
  log "Trying release binary ${url}"
  if curl -fsSL "$url" -o "$tmp"; then
    install -m 755 "$tmp" "$dest"
    rm -f "$tmp"
    ok "Installed $dest from ${tag}"
    return
  fi
  rm -f "$tmp"
  log "Release asset missing — building from source (needs Go + Node)…"
  command -v go >/dev/null 2>&1 || die "go toolchain required to build (or publish release assets for ${tag})"
  command -v npm >/dev/null 2>&1 || die "npm required to build embedded UI"
  (
    cd "$SOURCE_DIR"
    make build
  )
  [[ -x "$SOURCE_DIR/control-plane/bin/norix-control-plane" ]] || die "make build did not produce control-plane binary"
  install -m 755 "$SOURCE_DIR/control-plane/bin/norix-control-plane" "$dest"
  ok "Installed $dest from source build"
}

write_env_bare() {
  local public_url app_pw owner_pw secrets_key
  public_url="$(detect_public_url)"
  mkdir -p /etc/norix /var/lib/norix/backups
  useradd --system --home /var/lib/norix --shell /usr/sbin/nologin norix 2>/dev/null || true
  chown -R norix:norix /var/lib/norix

  if [[ -f "$BARE_ENV" ]]; then
    log "Keeping existing $BARE_ENV (upgrade-safe)"
    # Also keep a copy under data dir for operators
    mkdir -p "$SOURCE_DIR"
    cp -a "$BARE_ENV" "$ENV_FILE" 2>/dev/null || true
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
CORS_ALLOWED_ORIGINS=*
NORIX_PUBLIC_URL=${public_url}
NORIX_WEB_ORIGIN=${public_url}
NORIX_BACKUP_DIR=/var/lib/norix/backups
NORIX_BUILD_SECRETS_KEY=${secrets_key}
NORIX_REGISTRY=none
NORIX_INSTALL_MODE=bare
EOF
  chmod 600 "$BARE_ENV"
  chown norix:norix "$BARE_ENV"
  mkdir -p "$SOURCE_DIR"
  cp -a "$BARE_ENV" "$ENV_FILE"
  chmod 600 "$ENV_FILE"
  ok "Wrote $BARE_ENV"
}

install_systemd_bare() {
  local unit_src="${SOURCE_DIR}/deploy/systemd/norix-control-plane.bare.service"
  [[ -f "$unit_src" ]] || unit_src="${SOURCE_DIR}/deploy/systemd/norix-control-plane.service"
  [[ -f "$unit_src" ]] || die "missing systemd unit under deploy/systemd/"
  cp "$unit_src" /etc/systemd/system/norix-control-plane.service
  systemctl daemon-reload
  systemctl enable --now norix-control-plane
  ok "systemd unit norix-control-plane enabled"
}

# ── shared ─────────────────────────────────────────────────────────────────

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
  if [[ "$NORIX_MODE" == "bare" ]]; then
    die "not healthy — check: journalctl -u norix-control-plane -e"
  fi
  die "not healthy — check: docker compose -f $SOURCE_DIR/$COMPOSE_FILE logs control-plane"
}

print_next() {
  local public_url mode_note
  if [[ -f "$ENV_FILE" ]]; then
    public_url="$(grep -E '^NORIX_PUBLIC_URL=' "$ENV_FILE" | cut -d= -f2-)"
  elif [[ -f "$BARE_ENV" ]]; then
    public_url="$(grep -E '^NORIX_PUBLIC_URL=' "$BARE_ENV" | cut -d= -f2-)"
  else
    public_url="$(detect_public_url)"
  fi

  if [[ "$NORIX_MODE" == "bare" ]]; then
    mode_note="Mode: bare (no Docker for the control plane).
  To run container workloads on this host later, install Docker and the agent,
  or add worker nodes — see docs/runbooks/BARE_INSTALL.md."
  else
    mode_note="Mode: docker (Compose stack under ${SOURCE_DIR})."
  fi

  cat <<EOF

$(printf '\033[32m✔\033[0m') Norix is installed (beta).

  UI / API:   ${public_url}
  Health:     http://127.0.0.1:${NORIX_HTTP_PORT}/healthz
  Data:       ${NORIX_DATA}
  ${mode_note}

Next:
  1. Open the UI and create the first admin (bootstrap).
  2. Deploy a workload (container deploys need Docker on a node).
  3. Add workers with scripts/install-agent.sh when you scale out.

Upgrade:
  curl -fsSL ${NORIX_GITHUB_RAW}/${NORIX_REF}/scripts/get-norix.sh \\
    | NORIX_REF=${NORIX_REF} bash -s -- --mode ${NORIX_MODE}

Back up env secrets off-box (${ENV_FILE} and/or ${BARE_ENV}).

EOF
}

# ── main ───────────────────────────────────────────────────────────────────
log "Norix installer (mode=${NORIX_MODE}, ref=${NORIX_REF})"
sync_source

if [[ "$NORIX_MODE" == "docker" ]]; then
  install_docker
  write_env_docker
  # shellcheck disable=SC1090
  set -a && source "$ENV_FILE" && set +a
  NORIX_HTTP_PORT="${NORIX_HTTP_PORT:-8000}"
  start_stack_docker
else
  install_postgres_bare
  write_env_bare
  # shellcheck disable=SC1090
  set -a && source "$BARE_ENV" && set +a
  # HTTP_ADDR may be 0.0.0.0:8000 — extract port if NORIX_HTTP_PORT unset in file
  NORIX_HTTP_PORT="${NORIX_HTTP_PORT:-8000}"
  install_control_plane_binary_bare
  install_systemd_bare
fi

wait_health
print_next
