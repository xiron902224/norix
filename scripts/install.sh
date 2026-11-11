#!/usr/bin/env bash
set -Eeuo pipefail

NORIX_HOME="${NORIX_HOME:-${HOME}/.norix}"
NORIX_REPOSITORY="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${NORIX_REPOSITORY}/deploy/norix/docker-compose.yml"

log() { printf '[norix] %s\n' "$*" >&2; }
die() { log "ERROR: $*"; exit 1; }

require_command() {
    command -v "$1" >/dev/null 2>&1 || die "Required command not found: $1"
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
EOF
fi
chmod 600 "${ENV_FILE}"

log "Starting Norix with configuration in ${NORIX_HOME}"
docker compose --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" up -d

docker compose --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" ps
log "Norix is available at http://127.0.0.1:8080"
log "Health check: curl http://127.0.0.1:8080/healthz"
