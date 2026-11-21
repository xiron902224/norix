#!/usr/bin/env bash
# Mesh connectivity contract smoke (#48). Does not require live WireGuard CAP_NET_ADMIN.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT/control-plane"
echo "==> unit: workload DNS ↔ overlay AllowedIPs"
go test ./internal/network/ -run 'Connectivity|GenerateKeyPair' -count=1
echo "==> unit: NetworkApplyEnabled default on / opt-out"
go test ./internal/db/ -run NetworkApplyEnabled -count=1
echo "==> OK mesh connectivity contract (#48)"
