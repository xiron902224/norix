#!/usr/bin/env bash
set -Eeuo pipefail

NORIX_HOME="${NORIX_HOME:-${HOME}/.norix}"
NORIX_REPOSITORY="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${NORIX_REPOSITORY}/deploy/norix/docker-compose.yml"
COMMAND="${1:-up}"
if [[ $# -gt 0 ]]; then shift; fi

# Release verification defaults (GitHub Releases SHA256SUMS + optional local assets)
NORIX_GITHUB_REPO="${NORIX_GITHUB_REPO:-xiron902224/norix}"
NORIX_VERSION="${NORIX_VERSION:-}"
NORIX_SHA256SUMS="${NORIX_SHA256SUMS:-}"
NORIX_SHA256SUMS_URL="${NORIX_SHA256SUMS_URL:-}"

log() { printf '[norix] %s\n' "$*" >&2; }
die() { log "ERROR: $*"; exit 1; }

require_command() {
    command -v "$1" >/dev/null 2>&1 || die "Required command not found: $1"
}

usage() {
    cat <<'EOF'
Usage: scripts/install.sh [up|preflight|upgrade|uninstall|checksums|verify-release|status] [options]

  preflight       Read-only host checks (Docker, Compose, ports, disk, RAM, KVM); same as norix doctor
  up              Run preflight, then start postgres + edge (caddy/pebble); create ~/.norix/.env if missing
                  NORIX_SKIP_PREFLIGHT=1 skips the checks; NORIX_PREFLIGHT_PORTS sets the ports
                  (default 5432,14000,8088,8443,2019: what up publishes on 127.0.0.1)
  upgrade         Pull images and recreate services (preserves volumes)
  uninstall       Stop and remove containers (keeps volumes unless NORIX_PURGE=1)
  checksums       Print SHA256 of key binaries/scripts for release verification
  verify-release  Verify local binaries against a release SHA256SUMS file/URL
  status          Show compose service status

verify-release options / env:
  --version vX.Y.Z     Release tag (or NORIX_VERSION / VERSION file)
  --sums PATH|URL      SHA256SUMS path or URL (or NORIX_SHA256SUMS / NORIX_SHA256SUMS_URL)
  --repo owner/name    GitHub repo for asset URLs (default: NORIX_GITHUB_REPO)
  NORIX_RELEASE_DIR    If set, also run sha256sum -c --ignore-missing in that directory

Example:
  NORIX_VERSION=v1.0.0 ./scripts/install.sh verify-release
  ./scripts/install.sh verify-release --sums ./SHA256SUMS
EOF
}

detect_arch() {
    local m
    m="$(uname -m)"
    case "$m" in
        x86_64|amd64) echo amd64 ;;
        aarch64|arm64) echo arm64 ;;
        *) die "unsupported architecture: $m" ;;
    esac
}

resolve_version_tag() {
    local v="${NORIX_VERSION}"
    if [[ -z "$v" && -f "${NORIX_REPOSITORY}/VERSION" ]]; then
        v="$(tr -d '[:space:]' <"${NORIX_REPOSITORY}/VERSION")"
    fi
    [[ -n "$v" ]] || die "set NORIX_VERSION or --version (e.g. v1.0.0)"
    case "$v" in
        v*) echo "$v" ;;
        *) echo "v${v}" ;;
    esac
}

fetch_sha256sums() {
    local dest="$1"
    local src="${NORIX_SHA256SUMS}"
    local url="${NORIX_SHA256SUMS_URL}"
    if [[ -n "$src" && -f "$src" ]]; then
        cp "$src" "$dest"
        return
    fi
    if [[ -z "$url" && -n "$src" && "$src" =~ ^https?:// ]]; then
        url="$src"
    fi
    if [[ -z "$url" ]]; then
        local tag
        tag="$(resolve_version_tag)"
        url="https://github.com/${NORIX_GITHUB_REPO}/releases/download/${tag}/SHA256SUMS"
    fi
    require_command curl
    log "Fetching SHA256SUMS from ${url}"
    curl -fsSL "$url" -o "$dest"
}

verify_release_checksums() {
    require_command sha256sum
    local arch sums tmpdir
    arch="$(detect_arch)"
    tmpdir="$(mktemp -d)"
    # shellcheck disable=SC2064
    trap "rm -rf '${tmpdir}'" RETURN
    sums="${tmpdir}/SHA256SUMS"
    fetch_sha256sums "$sums"

    local expected=(
        "norix-control-plane-linux-${arch}"
        "norix-agent-linux-${arch}"
        "norix-cli-linux-${arch}"
    )
    local local_bins=(
        "${NORIX_REPOSITORY}/control-plane/bin/norix-control-plane"
        "${NORIX_REPOSITORY}/agent/target/release/norix-agent"
        "${NORIX_REPOSITORY}/cli/bin/norix-cli"
    )

    local failed=0
    local i name path want got line
    for i in "${!expected[@]}"; do
        name="${expected[$i]}"
        path="${local_bins[$i]}"
        line="$(grep -E "  ${name}\$" "$sums" || true)"
        if [[ -z "$line" ]]; then
            log "SHA256SUMS has no entry for ${name}"
            failed=1
            continue
        fi
        want="${line%% *}"
        if [[ ! -f "$path" ]]; then
            log "missing local binary for ${name}: ${path}"
            failed=1
            continue
        fi
        got="$(sha256sum "$path" | awk '{print $1}')"
        if [[ "$got" != "$want" ]]; then
            log "checksum mismatch for ${name}: local=${got} release=${want}"
            failed=1
        else
            log "OK ${name} (${got})"
        fi
    done

    if [[ -n "${NORIX_RELEASE_DIR:-}" && -d "${NORIX_RELEASE_DIR}" ]]; then
        log "Verifying files under NORIX_RELEASE_DIR=${NORIX_RELEASE_DIR}"
        (
            cd "${NORIX_RELEASE_DIR}"
            sha256sum -c "$sums" --ignore-missing
        ) || failed=1
    fi

    [[ "$failed" -eq 0 ]] || die "release checksum verification failed"
    log "Release checksum verification passed"
}

require_docker() {
    require_command docker
    require_command openssl
    docker info >/dev/null 2>&1 || die "Docker is installed but the current user cannot access it"
    docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is required"
}

# Read-only host checks, the same ones as `norix doctor` (#34).
# Hard failures stop the install. Soft findings only warn.
# NORIX_PREFLIGHT_PORTS overrides the ports. The default is what `up` publishes
# on 127.0.0.1: postgres 5432, pebble 14000, edge 8088/8443 and admin 2019.
# NORIX_SKIP_PREFLIGHT=1 skips the checks entirely.
preflight() {
    local failed=0
    local ports="${NORIX_PREFLIGHT_PORTS:-5432,14000,8088,8443,2019}"
    local port avail_kib mem_kib

    if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
        log "[PASS] docker: engine $(docker info --format '{{.ServerVersion}}' 2>/dev/null)"
    else
        log "[FAIL] docker: not installed, or the daemon is not reachable for this user"
        log "       fix: install Docker Engine (https://docs.docker.com/engine/install/) and add your user to the docker group"
        failed=1
    fi

    if docker compose version --short 2>/dev/null | grep -Eq '^v?[2-9]\.'; then
        log "[PASS] docker compose: $(docker compose version --short 2>/dev/null)"
    else
        log "[FAIL] docker compose: v2 is required"
        log "       fix: install the docker-compose-plugin package"
        failed=1
    fi

    if grep -qw overlay /proc/filesystems 2>/dev/null; then
        log "[PASS] kernel overlay: available"
    else
        log "[FAIL] kernel overlay: not available"
        log "       fix: sudo modprobe overlay (Docker's default storage driver)"
        failed=1
    fi

    IFS=',' read -r -a port_list <<<"${ports}"
    for port in "${port_list[@]}"; do
        if docker ps --format '{{.Ports}}' 2>/dev/null | grep -Eq "[:.]${port}->"; then
            log "[PASS] port ${port}: published by a running container (re-run is fine)"
        elif (exec 3<>"/dev/tcp/127.0.0.1/${port}") 2>/dev/null; then
            log "[FAIL] port ${port}: already in use"
            log "       fix: stop the process that owns it (ss -ltnp) or set NORIX_PREFLIGHT_PORTS"
            failed=1
        else
            log "[PASS] port ${port}: free"
        fi
    done

    avail_kib="$(df -Pk /var/lib/docker 2>/dev/null | awk 'NR==2 {print $4}')"
    [[ -n "${avail_kib}" ]] || avail_kib="$(df -Pk / 2>/dev/null | awk 'NR==2 {print $4}')"
    if [[ -z "${avail_kib}" ]]; then
        log "[WARN] disk: could not read free space"
        log "       fix: run df -h and confirm there is room for images and the build cache"
    elif (( avail_kib / 1024 / 1024 >= 10 )); then
        log "[PASS] disk: $(( avail_kib / 1024 / 1024 )) GiB free"
    else
        log "[WARN] disk: $(( avail_kib / 1024 / 1024 )) GiB free, 10 GiB recommended"
        log "       fix: free disk space; images and build cache need room"
    fi

    mem_kib="$(awk '/MemAvailable/ {print $2}' /proc/meminfo 2>/dev/null || true)"
    if [[ -n "${mem_kib}" ]] && (( mem_kib / 1024 >= 2048 )); then
        log "[PASS] memory: $(( mem_kib / 1024 )) MiB available"
    else
        log "[WARN] memory: under 2 GiB available, or not measurable"
        log "       fix: add RAM or stop other workloads before running builds"
    fi

    if [[ -r /dev/kvm && -w /dev/kvm ]]; then
        log "[PASS] kvm: /dev/kvm is accessible"
    else
        log "[WARN] kvm: /dev/kvm not accessible (optional; needed for the micro-VM runtime)"
    fi

    if (( failed )); then
        die "preflight failed; fix the [FAIL] items above (or set NORIX_SKIP_PREFLIGHT=1)"
    fi
    log "Preflight passed"
}

ensure_home() {
    if [[ -e "${NORIX_HOME}" && ! -d "${NORIX_HOME}" ]]; then
        die "NORIX_HOME exists but is not a directory: ${NORIX_HOME}"
    fi
    mkdir -p "${NORIX_HOME}"
    chmod 700 "${NORIX_HOME}"
}

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
    preflight)
        preflight
        ;;
    up|install)
        if [[ "${NORIX_SKIP_PREFLIGHT:-}" == "1" ]]; then
            log "Skipping preflight (NORIX_SKIP_PREFLIGHT=1)"
        else
            preflight
        fi
        require_docker
        ensure_home
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
        require_docker
        ensure_home
        ensure_env
        log "Pulling images and recreating services"
        compose pull postgres pebble caddy || true
        compose up -d --force-recreate postgres pebble caddy
        compose ps
        log "Upgrade complete (volumes preserved)"
        ;;
    uninstall|down)
        require_docker
        ensure_home
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
        require_docker
        ensure_home
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
            "${NORIX_REPOSITORY}/cli/bin/norix-cli"
        )
        for path in "${targets[@]}"; do
            if [[ -f "${path}" ]]; then
                sha256sum "${path}"
            else
                log "missing: ${path}"
            fi
        done
        ;;
    verify-release)
        while [[ $# -gt 0 ]]; do
            case "$1" in
                --version) NORIX_VERSION="$2"; shift 2 ;;
                --sums) NORIX_SHA256SUMS="$2"; shift 2 ;;
                --repo) NORIX_GITHUB_REPO="$2"; shift 2 ;;
                -h|--help) usage; exit 0 ;;
                *) die "unknown verify-release option: $1" ;;
            esac
        done
        verify_release_checksums
        ;;
    *)
        usage
        die "unknown command: ${COMMAND}"
        ;;
esac
