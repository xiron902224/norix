#!/usr/bin/env bash
set -euo pipefail

CONTROL_PLANE="${NORIX_CONTROL_PLANE_URL:-http://127.0.0.1:8080}"
JOIN_TOKEN="${NORIX_JOIN_TOKEN:-}"
NODE_NAME="${NORIX_NODE_NAME:-}"
INSTALL_DIR="${NORIX_AGENT_INSTALL_DIR:-/usr/local/bin}"
WORKDIR="${NORIX_AGENT_WORKDIR:-/var/lib/norix-agent}"
BUILD_FROM_SOURCE=0
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

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
  -h, --help            Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --control-plane) CONTROL_PLANE="$2"; shift 2 ;;
    --token) JOIN_TOKEN="$2"; shift 2 ;;
    --name) NODE_NAME="$2"; shift 2 ;;
    --workdir) WORKDIR="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --from-source) BUILD_FROM_SOURCE=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

if [[ -z "$JOIN_TOKEN" ]]; then
  echo "join token required (--token or NORIX_JOIN_TOKEN)" >&2
  exit 1
fi

if [[ -z "$NODE_NAME" ]]; then
  NODE_NAME="$(hostname -s 2>/dev/null || hostname)"
fi

mkdir -p "$WORKDIR"

if [[ "$BUILD_FROM_SOURCE" -eq 1 ]]; then
  if ! command -v cargo >/dev/null 2>&1; then
    echo "cargo is required for --from-source" >&2
    exit 1
  fi
  echo "building norix-agent from source..."
  (cd "$REPO_ROOT/agent" && cargo build --release)
  install -m 0755 "$REPO_ROOT/agent/target/release/norix-agent" "$INSTALL_DIR/norix-agent"
else
  if [[ -x "$REPO_ROOT/agent/target/release/norix-agent" ]]; then
    install -m 0755 "$REPO_ROOT/agent/target/release/norix-agent" "$INSTALL_DIR/norix-agent"
  elif command -v cargo >/dev/null 2>&1; then
    echo "release binary missing; building from source..."
    (cd "$REPO_ROOT/agent" && cargo build --release)
    install -m 0755 "$REPO_ROOT/agent/target/release/norix-agent" "$INSTALL_DIR/norix-agent"
  else
    echo "norix-agent binary not found and cargo is unavailable" >&2
    exit 1
  fi
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

echo "installed $INSTALL_DIR/norix-agent"
echo "state dir: $WORKDIR"
echo "starting agent..."
# shellcheck disable=SC1090
set -a
source "$WORKDIR/agent.env"
set +a
exec "$INSTALL_DIR/norix-agent"
