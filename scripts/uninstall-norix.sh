#!/usr/bin/env bash
# Uninstall Norix control plane from this host (bare or docker compose).
set -euo pipefail

MODE="${NORIX_MODE:-auto}"
PURGE=0
NORIX_GITHUB_REPO="${NORIX_GITHUB_REPO:-xiron902224/norix}"
REPO_ROOT=""
if [[ -n "${BASH_SOURCE[0]:-}" ]]; then
  REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." 2>/dev/null && pwd || true)"
fi

BARE_ENV="/etc/norix/control-plane.env"
INSTALL_STATE="/var/lib/norix/install-state.json"
BARE_PID_FILE="/var/lib/norix/norix-control-plane.pid"
BARE_BIN="/usr/local/bin/norix-control-plane"

usage() {
  cat <<'EOF'
Usage: uninstall-norix.sh [options]

Options:
  --mode auto|bare|docker   Detect from install-state when auto (default)
  --purge                   Also remove /var/lib/norix, /etc/norix (not Postgres data unless bare+purge-db)
  --purge-db                Drop local Postgres DB/roles created by bare installer (destructive)
  -h, --help

Docker mode delegates to: scripts/install.sh uninstall
EOF
}

die() { echo "$*" >&2; exit 1; }
log() { echo "$*"; }

PURGE_DB=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) MODE="$2"; shift 2 ;;
    --purge) PURGE=1; shift ;;
    --purge-db) PURGE_DB=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

detect_mode() {
  if [[ "$MODE" != "auto" ]]; then
    echo "$MODE"
    return
  fi
  if [[ -f "$INSTALL_STATE" ]] && grep -q '"mode": "bare"' "$INSTALL_STATE" 2>/dev/null; then
    echo bare
  elif [[ -f "$BARE_ENV" || -x "$BARE_BIN" ]]; then
    echo bare
  else
    echo docker
  fi
}

MODE="$(detect_mode)"
log "uninstalling Norix control plane (mode=${MODE})…"

uninstall_bare() {
  if command -v systemctl >/dev/null 2>&1 && [[ "$(cat /proc/1/comm 2>/dev/null || true)" == "systemd" ]]; then
    systemctl disable --now norix-control-plane 2>/dev/null || true
    rm -f /etc/systemd/system/norix-control-plane.service
    systemctl daemon-reload 2>/dev/null || true
    log "stopped systemd unit norix-control-plane"
  fi
  if [[ -f "$BARE_PID_FILE" ]]; then
    old="$(cat "$BARE_PID_FILE" 2>/dev/null || true)"
    if [[ -n "$old" ]] && kill -0 "$old" 2>/dev/null; then
      log "stopping PID $old"
      kill "$old" 2>/dev/null || true
      sleep 1
      kill -9 "$old" 2>/dev/null || true
    fi
    rm -f "$BARE_PID_FILE"
  fi
  pkill -f "$BARE_BIN" 2>/dev/null || true
  rm -f "$BARE_BIN"
  log "removed $BARE_BIN"

  if [[ "$PURGE_DB" -eq 1 ]]; then
    log "purging local Postgres roles/db (best-effort)…"
    if command -v sudo >/dev/null 2>&1; then
      sudo -u postgres psql -v ON_ERROR_STOP=0 -c "DROP DATABASE IF EXISTS norix;" 2>/dev/null || true
      sudo -u postgres psql -v ON_ERROR_STOP=0 -c "DROP ROLE IF EXISTS norix_app; DROP ROLE IF EXISTS norix;" 2>/dev/null || true
    else
      su - postgres -c "psql -c 'DROP DATABASE IF EXISTS norix;'" 2>/dev/null || true
      su - postgres -c "psql -c 'DROP ROLE IF EXISTS norix_app; DROP ROLE IF EXISTS norix;'" 2>/dev/null || true
    fi
  fi

  if [[ "$PURGE" -eq 1 ]]; then
    rm -rf /var/lib/norix /etc/norix
    log "purged /var/lib/norix and /etc/norix"
  else
    log "kept /var/lib/norix and /etc/norix (pass --purge to delete)"
  fi
}

uninstall_docker() {
  if [[ -n "$REPO_ROOT" && -x "$REPO_ROOT/scripts/install.sh" ]]; then
    if [[ "$PURGE" -eq 1 ]]; then
      NORIX_PURGE=1 "$REPO_ROOT/scripts/install.sh" uninstall
    else
      "$REPO_ROOT/scripts/install.sh" uninstall
    fi
  else
    die "docker uninstall needs repo scripts/install.sh (clone the repo or use --mode bare)"
  fi
}

case "$MODE" in
  bare) uninstall_bare ;;
  docker) uninstall_docker ;;
  *) die "invalid mode: $MODE" ;;
esac

log "control-plane uninstall complete"
log "docs: https://github.com/${NORIX_GITHUB_REPO}/blob/main/docs/UPDATE_AND_UNINSTALL.md"
