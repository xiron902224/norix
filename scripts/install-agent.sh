#!/usr/bin/env bash
set -euo pipefail

CONTROL_PLANE="${NORIX_CONTROL_PLANE_URL:-http://127.0.0.1:8080}"
JOIN_TOKEN="${NORIX_JOIN_TOKEN:-}"
NODE_NAME="${NORIX_NODE_NAME:-}"
INSTALL_DIR="${NORIX_AGENT_INSTALL_DIR:-/usr/local/bin}"
WORKDIR="${NORIX_AGENT_WORKDIR:-/var/lib/norix-agent}"
BUILD_FROM_SOURCE=0
SKIP_VERIFY=0
NO_START=0
FOREGROUND=0
NORIX_SUPERVISE="${NORIX_SUPERVISE:-auto}"
NORIX_VERSION="${NORIX_VERSION:-}"
NORIX_GITHUB_REPO="${NORIX_GITHUB_REPO:-xiron902224/norix}"
NORIX_SHA256SUMS="${NORIX_SHA256SUMS:-}"
NORIX_AGENT_ASSET_URL="${NORIX_AGENT_ASSET_URL:-}"
# When piped via curl|bash, BASH_SOURCE may be unset under `set -u`.
REPO_ROOT=""
if [[ -n "${BASH_SOURCE[0]:-}" ]]; then
  REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." 2>/dev/null && pwd || true)"
fi
[[ -n "$REPO_ROOT" && -f "${REPO_ROOT}/VERSION" ]] || REPO_ROOT=""

usage() {
  cat <<'EOF'
Usage: install-agent.sh [options]

Options:
  --control-plane URL   Control plane base URL (default: http://127.0.0.1:8080)
  --token TOKEN         One-time join token from the dashboard
  --name NAME           Node name (default: hostname)
  --workdir PATH        Agent state directory (default: /var/lib/norix-agent)
  --install-dir PATH    Binary install directory (default: /usr/local/bin)
  --from-source         Build with cargo from this repository
  --version TAG         Download release binary for TAG (e.g. v1.0.0-beta.3); verifies SHA256SUMS
  --sums PATH|URL       SHA256SUMS file or URL (default: GitHub release asset for --version)
  --asset-url URL       Direct URL to norix-agent-linux-{amd64,arm64} binary
  --skip-verify         Skip SHA256SUMS verification (not recommended)
  --supervise MODE      auto|systemd|none (default: auto)
                        auto → systemd when PID 1 is systemd, else nohup (labs/containers)
                        systemd → require systemd unit
                        none → nohup + pid/log under workdir
  --foreground          Run agent in this terminal (exec; blocks). Overrides --supervise.
  --no-start            Install binary + agent.env (+ unit if systemd) without starting
  -h, --help            Show help

Env:
  NORIX_VERSION, NORIX_GITHUB_REPO, NORIX_SHA256SUMS, NORIX_AGENT_ASSET_URL, NORIX_SUPERVISE

Production (3 nodes): see docs/runbooks/BETA1_3NODE_INSTALL.md and deploy/systemd/.
EOF
}

die() { echo "$*" >&2; exit 1; }

has_working_systemd() {
  command -v systemctl >/dev/null 2>&1 || return 1
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
        echo none
      fi
      ;;
    *)
      die "invalid --supervise / NORIX_SUPERVISE: $NORIX_SUPERVISE (use auto|systemd|none)"
      ;;
  esac
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
  if [[ -z "$v" && -f "${REPO_ROOT}/VERSION" ]]; then
    v="$(tr -d '[:space:]' <"${REPO_ROOT}/VERSION")"
  fi
  # When installed via curl|bash there is no local VERSION — pin to published beta.
  if [[ -z "$v" ]]; then
    v="v1.0.0-beta.3"
  fi
  case "$v" in
    v*) echo "$v" ;;
    *) echo "v${v}" ;;
  esac
}

fetch_to() {
  local url="$1" dest="$2"
  command -v curl >/dev/null 2>&1 || die "curl is required to download release assets"
  curl -fsSL "$url" -o "$dest"
}

verify_agent_checksum() {
  local binary_path="$1" asset_name="$2"
  [[ "$SKIP_VERIFY" -eq 1 ]] && return 0
  command -v sha256sum >/dev/null 2>&1 || die "sha256sum is required for release verification (or pass --skip-verify)"

  local sums tmp line want got
  tmp="$(mktemp)"
  if [[ -n "${NORIX_SHA256SUMS}" && -f "${NORIX_SHA256SUMS}" ]]; then
    cp "${NORIX_SHA256SUMS}" "$tmp"
  elif [[ -n "${NORIX_SHA256SUMS}" && "${NORIX_SHA256SUMS}" =~ ^https?:// ]]; then
    fetch_to "${NORIX_SHA256SUMS}" "$tmp"
  else
    local tag
    tag="$(resolve_version_tag)"
    fetch_to "https://github.com/${NORIX_GITHUB_REPO}/releases/download/${tag}/SHA256SUMS" "$tmp"
  fi

  line="$(grep -E "  ${asset_name}\$" "$tmp" || true)"
  want="${line%% *}"
  got="$(sha256sum "$binary_path" | awk '{print $1}')"
  rm -f "$tmp"
  [[ -n "$line" ]] || die "SHA256SUMS has no entry for ${asset_name}"
  [[ "$got" == "$want" ]] || die "checksum mismatch for ${asset_name}: got=${got} want=${want}"
  echo "verified ${asset_name} sha256=${got}"
}

download_release_agent() {
  local arch asset_name url dest
  arch="$(detect_arch)"
  asset_name="norix-agent-linux-${arch}"
  dest="$(mktemp)"
  if [[ -n "${NORIX_AGENT_ASSET_URL}" ]]; then
    url="${NORIX_AGENT_ASSET_URL}"
  else
    local tag
    tag="$(resolve_version_tag)"
    url="https://github.com/${NORIX_GITHUB_REPO}/releases/download/${tag}/${asset_name}"
  fi
  echo "downloading ${url}"
  fetch_to "$url" "$dest"
  verify_agent_checksum "$dest" "$asset_name"
  install -m 0755 "$dest" "$INSTALL_DIR/norix-agent"
  rm -f "$dest"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --control-plane) CONTROL_PLANE="$2"; shift 2 ;;
    --token) JOIN_TOKEN="$2"; shift 2 ;;
    --name) NODE_NAME="$2"; shift 2 ;;
    --workdir) WORKDIR="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --from-source) BUILD_FROM_SOURCE=1; shift ;;
    --version) NORIX_VERSION="$2"; shift 2 ;;
    --sums) NORIX_SHA256SUMS="$2"; shift 2 ;;
    --asset-url) NORIX_AGENT_ASSET_URL="$2"; shift 2 ;;
    --skip-verify) SKIP_VERIFY=1; shift ;;
    --supervise)
      NORIX_SUPERVISE="${2:-}"
      [[ -n "$NORIX_SUPERVISE" ]] || die "--supervise requires auto|systemd|none"
      shift 2
      ;;
    --supervise=*)
      NORIX_SUPERVISE="${1#--supervise=}"
      shift
      ;;
    --foreground) FOREGROUND=1; shift ;;
    --no-start) NO_START=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

case "$NORIX_SUPERVISE" in
  auto|systemd|none) ;;
  *) die "invalid --supervise / NORIX_SUPERVISE: $NORIX_SUPERVISE (use auto|systemd|none)" ;;
esac

if [[ -z "$JOIN_TOKEN" ]]; then
  echo "join token required (--token or NORIX_JOIN_TOKEN)" >&2
  exit 1
fi

if [[ -z "$NODE_NAME" ]]; then
  NODE_NAME="$(hostname -s 2>/dev/null || hostname)"
fi

mkdir -p "$WORKDIR"
mkdir -p "$INSTALL_DIR"

if [[ "$BUILD_FROM_SOURCE" -eq 1 ]]; then
  if ! command -v cargo >/dev/null 2>&1; then
    echo "cargo is required for --from-source" >&2
    exit 1
  fi
  echo "building norix-agent from source..."
  (cd "$REPO_ROOT/agent" && cargo build --release)
  install -m 0755 "$REPO_ROOT/agent/target/release/norix-agent" "$INSTALL_DIR/norix-agent"
elif [[ -n "${NORIX_VERSION}" || -n "${NORIX_AGENT_ASSET_URL}" ]]; then
  download_release_agent
elif [[ -x "$REPO_ROOT/agent/target/release/norix-agent" ]]; then
  install -m 0755 "$REPO_ROOT/agent/target/release/norix-agent" "$INSTALL_DIR/norix-agent"
else
  # Default one-click path: download release binary from GitHub (no local cargo).
  NORIX_VERSION="$(resolve_version_tag)"
  echo "no local agent binary; downloading release ${NORIX_VERSION}…"
  download_release_agent
fi

cat >"$WORKDIR/agent.env" <<EOF
NORIX_CONTROL_PLANE_URL=$CONTROL_PLANE
NORIX_JOIN_TOKEN=$JOIN_TOKEN
NORIX_NODE_NAME=$NODE_NAME
NORIX_AGENT_WORKDIR=$WORKDIR
NORIX_HEARTBEAT_SECS=10
NORIX_AGENT_LOG_LEVEL=info
EOF
chmod 600 "$WORKDIR/agent.env"

AGENT_BIN="$INSTALL_DIR/norix-agent"
AGENT_ENV="$WORKDIR/agent.env"
AGENT_LOG="$WORKDIR/norix-agent.log"
AGENT_PID="$WORKDIR/norix-agent.pid"
UNIT_PATH="/etc/systemd/system/norix-agent.service"

echo "installed $AGENT_BIN"
echo "state dir: $WORKDIR"
echo "env file: $AGENT_ENV"

install_agent_unit() {
  local unit_src=""
  if [[ -n "$REPO_ROOT" && -f "$REPO_ROOT/deploy/systemd/norix-agent.service" ]]; then
    unit_src="$REPO_ROOT/deploy/systemd/norix-agent.service"
  fi
  if [[ -n "$unit_src" ]]; then
    cp "$unit_src" "$UNIT_PATH"
  else
    # curl|bash path: embed the same unit as deploy/systemd/norix-agent.service
    cat >"$UNIT_PATH" <<EOF
[Unit]
Description=Norix node agent
Documentation=https://github.com/${NORIX_GITHUB_REPO}
After=network-online.target docker.service
Wants=network-online.target
Requires=docker.service

[Service]
Type=simple
User=root
WorkingDirectory=${WORKDIR}
EnvironmentFile=-${AGENT_ENV}
ExecStart=${AGENT_BIN}
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
  fi
  # Keep EnvironmentFile / WorkingDirectory aligned with this install.
  if grep -q '^WorkingDirectory=' "$UNIT_PATH"; then
    sed -i "s|^WorkingDirectory=.*|WorkingDirectory=${WORKDIR}|" "$UNIT_PATH"
  fi
  if grep -q '^EnvironmentFile=' "$UNIT_PATH"; then
    sed -i "s|^EnvironmentFile=.*|EnvironmentFile=-${AGENT_ENV}|" "$UNIT_PATH"
  fi
  if grep -q '^ExecStart=' "$UNIT_PATH"; then
    sed -i "s|^ExecStart=.*|ExecStart=${AGENT_BIN}|" "$UNIT_PATH"
  fi
  systemctl daemon-reload
}

stop_agent_none_if_running() {
  if [[ -f "$AGENT_PID" ]]; then
    local old
    old="$(cat "$AGENT_PID" 2>/dev/null || true)"
    if [[ -n "$old" ]] && kill -0 "$old" 2>/dev/null; then
      echo "stopping previous norix-agent PID $old"
      kill "$old" 2>/dev/null || true
      sleep 1
      kill -9 "$old" 2>/dev/null || true
    fi
    rm -f "$AGENT_PID"
  fi
}

start_agent_systemd() {
  install_agent_unit
  systemctl enable --now norix-agent
  echo "norix-agent supervised by systemd (norix-agent.service)"
  systemctl --no-pager --full status norix-agent || true
}

start_agent_none() {
  stop_agent_none_if_running
  [[ -x "$AGENT_BIN" ]] || die "missing $AGENT_BIN"
  [[ -f "$AGENT_ENV" ]] || die "missing $AGENT_ENV"
  touch "$AGENT_LOG"
  echo "starting norix-agent under nohup (--supervise none)"
  set -a
  # shellcheck disable=SC1090
  source "$AGENT_ENV"
  set +a
  nohup "$AGENT_BIN" >>"$AGENT_LOG" 2>&1 &
  echo $! >"$AGENT_PID"
  chmod 644 "$AGENT_PID" 2>/dev/null || true
  sleep 1
  local pid stat
  pid="$(cat "$AGENT_PID" 2>/dev/null || true)"
  [[ -n "$pid" ]] || die "norix-agent failed to start (no PID) — see $AGENT_LOG"
  stat="$(ps -o stat= -p "$pid" 2>/dev/null | tr -d ' ' || true)"
  if [[ -z "$stat" || "$stat" == *Z* ]] || ! kill -0 "$pid" 2>/dev/null; then
    echo "last log lines ($AGENT_LOG):" >&2
    tail -n 40 "$AGENT_LOG" 2>/dev/null || true
    die "norix-agent exited immediately — see $AGENT_LOG"
  fi
  echo "norix-agent running (PID ${pid}, log ${AGENT_LOG})"
}

if [[ "$FOREGROUND" -eq 1 ]]; then
  if [[ "$NO_START" -eq 1 ]]; then
    die "--foreground and --no-start cannot be combined"
  fi
  echo "starting agent (foreground)..."
  # shellcheck disable=SC1090
  set -a
  source "$AGENT_ENV"
  set +a
  exec "$AGENT_BIN"
fi

SUPERVISE_MODE="$(resolve_supervise)"

if [[ "$NO_START" -eq 1 ]]; then
  case "$SUPERVISE_MODE" in
    systemd)
      install_agent_unit
      echo "(--no-start) unit installed at $UNIT_PATH"
      echo "  systemctl enable --now norix-agent"
      ;;
    none)
      echo "(--no-start) start later with:"
      echo "  set -a && . $AGENT_ENV && set +a"
      echo "  nohup $AGENT_BIN >>$AGENT_LOG 2>&1 & echo \$! >$AGENT_PID"
      ;;
  esac
  exit 0
fi

case "$SUPERVISE_MODE" in
  systemd) start_agent_systemd ;;
  none) start_agent_none ;;
  *) die "internal: unknown SUPERVISE_MODE=$SUPERVISE_MODE" ;;
esac
