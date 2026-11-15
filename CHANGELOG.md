# Changelog

## Unreleased

## 1.0.0-beta.4 — 2026-11-15

### Fixed
- Report real control-plane version via embedded `VERSION` / ldflags / `NORIX_VERSION` (no more hardcoded `1.0.0` fallback) (#71)
- Nodes UI shows agent version; local node reports `version+local`; agent crate version aligned to release (#71)
- Bare installer writes `NORIX_VERSION` into `control-plane.env` (#71)
- Release workflow: publish assets with sequential `gh release upload` (avoids softprops race that left beta drafts incomplete) (#62 / #70)

### Added
- GitHub pull request template + `type(#issue): …` title conventions (#74)
- In-place update tracking issue (#73)

### Changed
- Gitflow is **main-only**: branch from `main`, PR to `main` (`develop` retired)
- Default `NORIX_REF=main` in `get-norix.sh`; CI PRs target `main`
- Spanish collaborator docs: [`README.es.md`](README.es.md), [`CONTRIBUTING.es.md`](CONTRIBUTING.es.md)
- Docs / installer / Compose pins → `v1.0.0-beta.4`

### Notes
- Stage 0 trust/safety issues remain open (`ISSUES.md`)
- Tag `v1.0.0-beta.4` after merge to run the Release workflow (#75)

## 1.0.0-beta.3 — 2026-11-15

### Added
- `install-agent.sh --supervise auto|systemd|none` (default **auto**: systemd on real VPS, **nohup** in labs/containers); `--foreground` keeps the old blocking `exec`
- Published contribution backlog: [`ISSUES.md`](ISSUES.md) (triage of 55 open GitHub issues) + refreshed [`VISION.md`](VISION.md)

### Changed
- Bare installer one-click path: preflight, idempotent DB bootstrap, release binary **SHA256** verification, `--supervise auto|systemd|none`, install-state JSON
- Docs: README, [`BARE_INSTALL.md`](docs/runbooks/BARE_INSTALL.md), [`BETA1_3NODE_INSTALL.md`](docs/runbooks/BETA1_3NODE_INSTALL.md), [`ESTADO_Y_PLAN_V1.md`](docs/ESTADO_Y_PLAN_V1.md) pinned to `v1.0.0-beta.3`
- Dashboard join command / agent release pin → `v1.0.0-beta.3`

### Notes
- Stage 0 trust/safety GitHub issues remain open (see `ISSUES.md`); beta.3 does not claim Stage 0 exit
- Tag `v1.0.0-beta.3` after merge to run the Release workflow

## 1.0.0-beta.2 — 2026-11-15

### Added
- Installer `--mode bare`: system Postgres + control-plane binary + systemd (no Docker for the panel)
- Runbook [`docs/runbooks/BARE_INSTALL.md`](docs/runbooks/BARE_INSTALL.md)
- README focused on production install; local dev moved to CONTRIBUTING

### Changed
- Removed third-party PaaS comparisons from public docs
- `main` promoted as public tip alongside develop (this release)

## 1.0.0-beta.1 — 2026-11-15

First public **beta** pre-release (not GA). Semver pre-release of the 1.0 track.

### Added
- Full S0–S22 surface in code: auth/RLS, deploy image/git/**compose**, edge TLS, mesh, observability, managed DBs
- OIDC authorization-code (`/auth/oidc/*`) + real TOTP (RFC) with login enforce
- Control-plane **HA leases** wired (`ha.RunElection` → leader-only reconcile/ops/autoscaler)
- Autoscaler **applies** `replica_count` when `NORIX_AUTOSCALER=1`
- DNS-01 ACME via lego (`NORIX_ACME_DNS_PROVIDER` + `NORIX_ACME_EMAIL`)
- Managed DB dump artifacts (local `NORIX_BACKUP_DIR` + optional S3)
- Sandbox docker exec + warm pool; PR preview TTL/GC (migrations `000021`+)
- Wasm staging + optional WASI HTTP (`NORIX_WASM_SERVE=1`); micro-VM Cloud Hypervisor path
- SDKs: TypeScript, Python, Go
- Public `GET /status` + opt-in telemetry (`NORIX_TELEMETRY=1`)
- Patroni local stack: `deploy/postgresql/patroni/`
- Release workflow: multi-arch binaries, `SHA256SUMS`, keyless cosign (`.github/workflows/release.yml`)
- SBOM script, k6 smoke, chaos catalog, SECURITY/UPGRADE docs
- Apache-2.0 license; CONTRIBUTING + production install docs in README

### Beta caveats
- CI must be green before tagging `v1.0.0-beta.1`
- Several ops features are env-gated and need host validation (WG, KVM, DNS-01, Patroni, S3)
- GA / `v1.0.0` waits for stable E2E smoke on a reference host

### Ops notes
- WireGuard live apply: `NORIX_APPLY_WIREGUARD=1`
- Micro-VM boot: KVM + `NORIX_MICROVM_KERNEL` / `NORIX_MICROVM_ROOTFS`
- Publish: create git tag `v*` to run the Release workflow
