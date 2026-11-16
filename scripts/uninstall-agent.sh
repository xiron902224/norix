#!/usr/bin/env bash
# Remove norix-agent from this host (systemd unit and/or nohup pidfile).
# Does not remove Docker workloads unless --purge-workloads.
set -euo pipefail

INSTALL_DIR="${NORIX_AGENT_INSTALL_DIR:-/usr/local/bin}"
WORKDIR="${NORIX_AGENT_WORKDIR:-/var/lib/norix-agent}"
PURGE=0
PURGE_WORKLOADS=0
NORIX_GITHUB_REPO="${NORIX_GITHUB_REPO:-xiron902224/norix}"

usage() {
  cat <<'EOF'
Usage: uninstall-agent.sh [options]

Options:
  --workdir PATH         Agent state directory (default: /var/lib/norix-agent)
  --install-dir PATH     Binary directory (default: /usr/local/bin)
  --purge                Also delete workdir (node-state, logs, agent.env)
  --purge-workloads      Stop/remove containers labeled norix.workload_id=*
  -h, --help             Show help
EOF
}

die() { echo "$*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --workdir) WORKDIR="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --purge) PURGE=1; shift ;;
    --purge-workloads) PURGE_WORKLOADS=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

echo "uninstalling norix-agent…"

if command -v systemctl >/dev/null 2>&1 && [[ "$(cat /proc/1/comm 2>/dev/null || true)" == "systemd" ]]; then
  systemctl disable --now norix-agent 2>/dev/null || true
  rm -f /etc/systemd/system/norix-agent.service
  systemctl daemon-reload 2>/dev/null || true
  echo "stopped systemd unit norix-agent"
fi

PID_FILE="${WORKDIR}/norix-agent.pid"
ALT_PID="${WORKDIR}/agent.pid"
for f in "$PID_FILE" "$ALT_PID"; do
  if [[ -f "$f" ]]; then
    old="$(cat "$f" 2>/dev/null || true)"
    if [[ -n "$old" ]] && kill -0 "$old" 2>/dev/null; then
      echo "stopping PID $old"
      kill "$old" 2>/dev/null || true
      sleep 1
      kill -9 "$old" 2>/dev/null || true
    fi
    rm -f "$f"
  fi
done

# Best-effort: any remaining norix-agent processes for this binary
if [[ -x "${INSTALL_DIR}/norix-agent" ]]; then
  pkill -f "${INSTALL_DIR}/norix-agent" 2>/dev/null || true
fi

if [[ "$PURGE_WORKLOADS" -eq 1 ]] && command -v docker >/dev/null 2>&1; then
  ids="$(docker ps -aq --filter label=norix.workload_id 2>/dev/null || true)"
  if [[ -n "$ids" ]]; then
    echo "removing labeled workloads…"
    # shellcheck disable=SC2086
    docker rm -f $ids 2>/dev/null || true
  fi
fi

rm -f "${INSTALL_DIR}/norix-agent"
echo "removed ${INSTALL_DIR}/norix-agent"

if [[ "$PURGE" -eq 1 ]]; then
  rm -rf "$WORKDIR"
  echo "purged $WORKDIR"
else
  echo "kept state dir $WORKDIR (pass --purge to delete)"
fi

echo "norix-agent uninstall complete"
echo "docs: https://github.com/${NORIX_GITHUB_REPO}/blob/main/docs/UPDATE_AND_UNINSTALL.md"
