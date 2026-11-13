#!/usr/bin/env bash
# S19: generate lightweight SBOMs for Go and Rust modules.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${NORIX_SBOM_DIR:-$ROOT/dist/sbom}"
mkdir -p "$OUT"
(
  cd "$ROOT/control-plane"
  go list -m all >"$OUT/control-plane-modules.txt"
)
(
  cd "$ROOT/agent"
  cargo tree -e normal >"$OUT/agent-cargo-tree.txt" 2>/dev/null || cargo metadata --format-version 1 >"$OUT/agent-metadata.json"
)
echo "Wrote SBOMs under $OUT"
