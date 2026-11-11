#!/usr/bin/env bash
set -Eeuo pipefail

NORIX_HOME="${NORIX_HOME:-${HOME}/.norix}"
NORIX_REPOSITORY="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${NORIX_REPOSITORY}/deploy/norix/docker-compose.yml"
COMMAND="${1:-up}"

log() { printf '[norix] %s\n' "$*" >&2; }
die() { log "ERROR: $*"; exit 1; }

require_command() {
    command -v "$1" >/dev/null 2>&1 || die "Required command not found: $1"
}

usage() {
    cat <<'EOF'
Usage: scripts/install.sh [up|upgrade|uninstall|checksums|status]

  up         Start postgres + edge (caddy/pebble); create ~/.norix/.env if missing
  upgrade    Pull images and recreate services (preserves volumes)
  uninstall  Stop and remove containers (keeps volumes unless NORIX_PURGE=1)
  checksums  Print SHA256 of key binaries/scripts for release verification
  status     Show compose service status
EOF
}

require_command docker
require_command openssl

docker info >/dev/null 2>&1 || die "Docker is installed but the current user cannot access it"
docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is required"

if [[ -e "${NORIX_HOME}" && ! -d "${NORIX_HOME}" ]]; then
    die "NORIX_HOME exists but is not a directory: ${NORIX_HOME}"
fi
mkdir -p "${NORIX_HOME}"
chmod 700 "${NORIX_HOME}"

ENV_FILE="${NORIX_HOME}/.env"
ensure_env() {
    if [[ ! -f "${ENV_FILE}" ]]; then
        POSTGRES_PASSWORD="$(openssl rand -hex 32)"
        POSTGRES_APP_PASSWORD="$(openssl rand -hex 32)"
        cat >"${ENV_FILE}" <<EOF
POSTGRES_DB=norix
POSTGRES_USER=norix
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
POSTGRES_APP_PASSWORD=${POSTGRES_APP_PASSWORD}
DATABASE_URL=postgres://norix_app:${POSTGRES_APP_PASSWORD}@postgres:5432/norix?sslmode=disable
MIGRATION_DATABASE_URL=postgres://norix:${POSTGRES_PASSWORD}@postgres:5432/norix?sslmode=disable
HTTP_ADDR=:8080
NORIX_REGISTRY=127.0.0.1:5000
NORIX_CADDY_ADMIN_URL=http://127.0.0.1:2019
NORIX_EDGE_UPSTREAM_HOST=host.docker.internal
NORIX_ACME_CA=https://pebble:14000/dir
EOF
        chmod 600 "${ENV_FILE}"
        log "Wrote ${ENV_FILE}"
    fi
}

compose() {
    docker compose --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" "$@"
}

case "${COMMAND}" in
    -h|--help|help)
        usage
        ;;
    up|install)
        ensure_env
        log "Starting Norix dependencies (postgres, caddy, pebble)"
        compose up -d postgres pebble caddy
        compose ps
        log "Dependencies ready. Run the control-plane on the host:"
        log "  set -a && source ${ENV_FILE} && set +a"
        log "  # remap DB URLs to 127.0.0.1 for host process, then:"
        log "  cd control-plane && go run ./cmd/server"
        log "Health: curl http://127.0.0.1:8080/healthz"
        log "Edge admin: http://127.0.0.1:2019  HTTP:8088  HTTPS:8443"
        ;;
    upgrade)
        ensure_env
        log "Pulling images and recreating services"
        compose pull postgres pebble caddy || true
        compose up -d --force-recreate postgres pebble caddy
        compose ps
        log "Upgrade complete (volumes preserved)"
        ;;
    uninstall|down)
        ensure_env
        if [[ "${NORIX_PURGE:-}" == "1" ]]; then
            log "Removing containers and volumes"
            compose down -v
            rm -f "${ENV_FILE}"
        else
            log "Stopping containers (volumes kept; set NORIX_PURGE=1 to wipe)"
            compose down
        fi
        ;;
    status)
        ensure_env
        compose ps
        ;;
    checksums)
        require_command sha256sum
        targets=(
            "${NORIX_REPOSITORY}/scripts/install.sh"
            "${NORIX_REPOSITORY}/scripts/install-agent.sh"
            "${NORIX_REPOSITORY}/control-plane/bin/norix-control-plane"
            "${NORIX_REPOSITORY}/agent/target/release/norix-agent"
        )
        for path in "${targets[@]}"; do
            if [[ -f "${path}" ]]; then
                sha256sum "${path}"
            else
                log "missing: ${path}"
            fi
        done
        ;;
    *)
        usage
        die "unknown command: ${COMMAND}"
        ;;
esac
