#!/usr/bin/env bash
# Offline smoke for release integrity helpers (#5).
# Verifies fail-closed SHA256 mismatch behavior without network/cosign.
set -euo pipefail

die() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "OK: $*"; }

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

asset="norix-control-plane-linux-amd64"
printf 'fake-binary-contents\n' >"${tmpdir}/${asset}"
good="$(sha256sum "${tmpdir}/${asset}" | awk '{print $1}')"
printf '%s  %s\n' "$good" "$asset" >"${tmpdir}/SHA256SUMS.good"
printf '%s  %s\n' "$(printf 'wrong\n' | sha256sum | awk '{print $1}')" "$asset" >"${tmpdir}/SHA256SUMS.bad"

check_one() {
  local sums="$1"
  local expected
  expected="$(awk -v f="$asset" '$2 == f {print $1; exit}' "$sums")"
  [[ -n "$expected" ]] || return 2
  local actual
  actual="$(sha256sum "${tmpdir}/${asset}" | awk '{print $1}')"
  [[ "$expected" == "$actual" ]]
}

check_one "${tmpdir}/SHA256SUMS.good" || die "good checksum should match"
ok "matching SHA256SUMS accepted"

if check_one "${tmpdir}/SHA256SUMS.bad"; then
  die "bad checksum should not match"
fi
ok "mismatched SHA256SUMS rejected"

scripts_dir="$(cd "$(dirname "$0")" && pwd)"
grep -q 'verify_cosign_bundle' "${scripts_dir}/get-norix.sh" || die "verify_cosign_bundle missing from get-norix.sh"
grep -q 'verify_agent_cosign' "${scripts_dir}/install-agent.sh" || die "verify_agent_cosign missing from install-agent.sh"
ok "cosign helpers present in get-norix.sh and install-agent.sh"

grep -q 'NORIX_SKIP_COSIGN' "${scripts_dir}/get-norix.sh" || die "NORIX_SKIP_COSIGN missing"
grep -q 'NORIX_SKIP_COSIGN\|--skip-cosign' "${scripts_dir}/install-agent.sh" || die "skip-cosign missing from install-agent.sh"
ok "NORIX_SKIP_COSIGN / --skip-cosign knobs present"

echo "All release-verify smokes passed."
