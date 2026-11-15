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
NORIX_VERSION="${NORIX_VERSION:-}"
NORIX_GITHUB_REPO="${NORIX_GITHUB_REPO:-xiron902224/norix}"
NORIX_SHA256SUMS="${NORIX_SHA256SUMS:-}"
NORIX_AGENT_ASSET_URL="${NORIX_AGENT_ASSET_URL:-}"
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
  --version TAG         Download release binary for TAG (e.g. v1.0.0-beta.1); verifies SHA256SUMS
  --sums PATH|URL       SHA256SUMS file or URL (default: GitHub release asset for --version)
  --asset-url URL       Direct URL to norix-agent-linux-{amd64,arm64} binary
  --skip-verify         Skip SHA256SUMS verification (not recommended)
  --no-start            Install binary + agent.env only (for systemd); do not exec the agent
  -h, --help            Show help

Env:
  NORIX_VERSION, NORIX_GITHUB_REPO, NORIX_SHA256SUMS, NORIX_AGENT_ASSET_URL

Production (3 nodes): see docs/runbooks/BETA1_3NODE_INSTALL.md and deploy/systemd/.
EOF
}

die() { echo "$*" >&2; exit 1; }

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
  [[ -n "$v" ]] || die "set --version / NORIX_VERSION (e.g. v1.0.0)"
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
    --no-start) NO_START=1; shift ;;
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
elif command -v cargo >/dev/null 2>&1; then
  echo "release binary missing; building from source..."
  (cd "$REPO_ROOT/agent" && cargo build --release)
  install -m 0755 "$REPO_ROOT/agent/target/release/norix-agent" "$INSTALL_DIR/norix-agent"
else
  echo "norix-agent binary not found; pass --version vX.Y.Z to download a release, or --from-source" >&2
  exit 1
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
echo "env file: $WORKDIR/agent.env"

if [[ "$NO_START" -eq 1 ]]; then
  echo "(--no-start) enable systemd unit: deploy/systemd/norix-agent.service"
  echo "  systemctl enable --now norix-agent"
  exit 0
fi

echo "starting agent (foreground)..."
# shellcheck disable=SC1090
set -a
source "$WORKDIR/agent.env"
set +a
exec "$INSTALL_DIR/norix-agent"
