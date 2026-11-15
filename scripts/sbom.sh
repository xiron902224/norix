#!/usr/bin/env bash
# Generate SBOMs for Norix modules.
# Prefer CycloneDX JSON via syft when available:
#   https://github.com/anchore/syft  (e.g. curl -sSfL …/syft/main/install.sh | sh -s -- -b ~/.local/bin)
# Fallback: go list / cargo tree text inventories (still useful for release packaging).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${NORIX_SBOM_DIR:-$ROOT/dist/sbom}"
mkdir -p "$OUT"

have_syft=0
if command -v syft >/dev/null 2>&1; then
  have_syft=1
fi

if [[ "$have_syft" -eq 1 ]]; then
  echo "Generating CycloneDX SBOMs with syft..."
  syft "dir:${ROOT}/control-plane" -o cyclonedx-json >"$OUT/control-plane.cdx.json"
  syft "dir:${ROOT}/cli" -o cyclonedx-json >"$OUT/cli.cdx.json"
  syft "dir:${ROOT}/agent" -o cyclonedx-json >"$OUT/agent.cdx.json"
  # SPDX sidecars for tooling that prefers SPDX.
  syft "dir:${ROOT}/control-plane" -o spdx-json >"$OUT/control-plane.spdx.json" || true
  syft "dir:${ROOT}/agent" -o spdx-json >"$OUT/agent.spdx.json" || true
else
  echo "syft not found; writing module inventory fallbacks (install syft for CycloneDX)."
fi

# Always write lightweight inventories so releases have something without syft.
(
  cd "$ROOT/control-plane"
  go list -m all >"$OUT/control-plane-modules.txt"
)
(
  cd "$ROOT/cli"
  go list -m all >"$OUT/cli-modules.txt"
)
(
  cd "$ROOT/agent"
  if command -v cargo >/dev/null 2>&1; then
    cargo tree -e normal >"$OUT/agent-cargo-tree.txt" 2>/dev/null \
      || cargo metadata --format-version 1 >"$OUT/agent-metadata.json"
  else
    echo "cargo not available" >"$OUT/agent-cargo-tree.txt"
  fi
)

cat >"$OUT/README.md" <<'EOF'
# Norix SBOM

- `*.cdx.json` — CycloneDX 1.x JSON (produced when [syft](https://github.com/anchore/syft) is on PATH)
- `*.spdx.json` — SPDX JSON (optional; syft)
- `*-modules.txt` / `agent-cargo-tree.txt` — always-on dependency inventories

Release workflow packs this directory as `norix-sbom.tar.gz` and includes it in GitHub Releases.
EOF

echo "Wrote SBOMs under $OUT"
ls -la "$OUT"
