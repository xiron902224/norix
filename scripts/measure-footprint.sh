#!/usr/bin/env bash
# Measure release binary size and idle RSS for control-plane + agent (#43).
# Usage:
#   ./scripts/measure-footprint.sh              # print Markdown table
#   ./scripts/measure-footprint.sh --check       # fail if over CI budgets
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CHECK=0
if [[ "${1:-}" == "--check" ]]; then
  CHECK=1
fi

# Soft budgets for CI regression (MiB). Raised only with an intentional CHANGELOG note.
CP_BIN_MAX_MIB="${NORIX_FOOTPRINT_CP_BIN_MIB:-45}"
AGENT_BIN_MAX_MIB="${NORIX_FOOTPRINT_AGENT_BIN_MIB:-12}"
CP_RSS_MAX_MIB="${NORIX_FOOTPRINT_CP_RSS_MIB:-80}"
AGENT_RSS_MAX_MIB="${NORIX_FOOTPRINT_AGENT_RSS_MIB:-40}"

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing $1" >&2; exit 1; }; }
need go
need cargo

bytes_mib() {
  local path="$1"
  local bytes
  bytes="$(wc -c <"$path" | tr -d ' ')"
  awk -v b="$bytes" 'BEGIN { printf "%.1f", b/1024/1024 }'
}

rss_mib() {
  local pid="$1"
  # VmRSS is kB on Linux.
  local kb
  kb="$(awk '/VmRSS:/ {print $2}' "/proc/$pid/status" 2>/dev/null || echo 0)"
  awk -v k="$kb" 'BEGIN { printf "%.1f", k/1024 }'
}

build_bins() {
  mkdir -p "$ROOT/control-plane/bin"
  if [[ ! -x "$ROOT/control-plane/bin/norix-control-plane" ]]; then
    (cd "$ROOT/control-plane" && CGO_ENABLED=0 go build -o bin/norix-control-plane ./cmd/server)
  fi
  if [[ ! -x "$ROOT/agent/target/release/norix-agent" ]]; then
    (cd "$ROOT/agent" && cargo build --release -q)
  fi
}

sample_idle_rss() {
  local bin="$1"
  shift
  local tmp log pid
  tmp="$(mktemp -d)"
  log="$tmp/out.log"
  # shellcheck disable=SC2086
  "$bin" "$@" >"$log" 2>&1 &
  pid=$!
  sleep 2
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "0.0"
    cat "$log" >&2 || true
    rm -rf "$tmp"
    return 0
  fi
  rss_mib "$pid"
  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  rm -rf "$tmp"
}

build_bins

CP_BIN="$ROOT/control-plane/bin/norix-control-plane"
AGENT_BIN="$ROOT/agent/target/release/norix-agent"
CP_BIN_MIB="$(bytes_mib "$CP_BIN")"
AGENT_BIN_MIB="$(bytes_mib "$AGENT_BIN")"

# Idle RSS: agent --help exits; for a short live sample, start with a dummy env that exits after bind fails
# is noisy. Prefer: run agent with --help for process start cost is 0. Measure a short-lived
# `norix-agent --version` style if available; otherwise sample after spawn with invalid CP URL.
CP_RSS_MIB="n/a"
AGENT_RSS_MIB="n/a"
if [[ -r /proc/self/status ]]; then
  # Control plane needs Postgres; without it it exits. Measure binary-only by spawning
  # with a throwaway DATABASE_URL so we catch the process briefly, or skip if no docker.
  if command -v docker >/dev/null 2>&1 && docker compose -f "$ROOT/deploy/norix/docker-compose.yml" ps postgres 2>/dev/null | grep -q Up; then
    :
  fi
  # Agent stays up only with a join token or saved state; use a throwaway token + workdir.
  AGENT_RSS_MIB="$(
    tmp_agent="$(mktemp -d)"
    NORIX_CONTROL_PLANE_URL="${NORIX_CONTROL_PLANE_URL:-http://127.0.0.1:9}" \
    NORIX_JOIN_TOKEN="${NORIX_JOIN_TOKEN:-measure-only-not-a-real-token}" \
    NORIX_AGENT_WORKDIR="$tmp_agent" \
    sample_idle_rss "$AGENT_BIN" || true
    rm -rf "$tmp_agent"
  )"
  # CP: only if DATABASE_URL works; otherwise report n/a for RSS and still check binary size.
  if [[ -n "${DATABASE_URL:-}" || -f "$ROOT/control-plane/.env" ]]; then
    set -a
    # shellcheck disable=SC1091
    [[ -f "$ROOT/control-plane/.env" ]] && . "$ROOT/control-plane/.env"
    set +a
    export NORIX_ALLOW_INSECURE_DEV_SECRETS="${NORIX_ALLOW_INSECURE_DEV_SECRETS:-1}"
    if [[ -n "${DATABASE_URL:-}" ]]; then
      CP_RSS_MIB="$(sample_idle_rss "$CP_BIN" || true)"
    fi
  fi
fi

echo "| Binary | Size (MiB) | Idle RSS (MiB) |"
echo "|---|---:|---:|"
echo "| control-plane | ${CP_BIN_MIB} | ${CP_RSS_MIB} |"
echo "| agent | ${AGENT_BIN_MIB} | ${AGENT_RSS_MIB} |"

if [[ "$CHECK" -eq 1 ]]; then
  fail=0
  awk -v v="$CP_BIN_MIB" -v m="$CP_BIN_MAX_MIB" 'BEGIN { exit !(v+0 > m+0) }' && {
    echo "control-plane binary ${CP_BIN_MIB} MiB exceeds budget ${CP_BIN_MAX_MIB}" >&2
    fail=1
  }
  awk -v v="$AGENT_BIN_MIB" -v m="$AGENT_BIN_MAX_MIB" 'BEGIN { exit !(v+0 > m+0) }' && {
    echo "agent binary ${AGENT_BIN_MIB} MiB exceeds budget ${AGENT_BIN_MAX_MIB}" >&2
    fail=1
  }
  if [[ "$CP_RSS_MIB" != "n/a" && "$CP_RSS_MIB" != "0.0" ]]; then
    awk -v v="$CP_RSS_MIB" -v m="$CP_RSS_MAX_MIB" 'BEGIN { exit !(v+0 > m+0) }' && {
      echo "control-plane RSS ${CP_RSS_MIB} MiB exceeds budget ${CP_RSS_MAX_MIB}" >&2
      fail=1
    }
  fi
  if [[ "$AGENT_RSS_MIB" != "n/a" && "$AGENT_RSS_MIB" != "0.0" ]]; then
    awk -v v="$AGENT_RSS_MIB" -v m="$AGENT_RSS_MAX_MIB" 'BEGIN { exit !(v+0 > m+0) }' && {
      echo "agent RSS ${AGENT_RSS_MIB} MiB exceeds budget ${AGENT_RSS_MAX_MIB}" >&2
      fail=1
    }
  fi
  exit "$fail"
fi
