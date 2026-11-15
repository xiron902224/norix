# Changelog

## 1.0.0 — 2026-11-15

### Added
- Full S0–S22 GA surface: auth/RLS, deploy image/git/**compose**, edge TLS, mesh, observability, managed DBs
- OIDC authorization-code (`/auth/oidc/*`) + real TOTP (RFC) with login enforce
- Control-plane **HA leases** wired (`ha.RunElection` → leader-only reconcile/ops/autoscaler)
- Autoscaler **applies** `replica_count` when `NORIX_AUTOSCALER=1`
- DNS-01 ACME via lego (`NORIX_ACME_DNS_PROVIDER` + `NORIX_ACME_EMAIL`)
- Managed DB real dump artifacts (local `NORIX_BACKUP_DIR` + optional S3)
- Sandbox docker exec + warm pool; PR preview TTL/GC (migrations `000021`+)
- Wasm staging + optional WASI HTTP (`NORIX_WASM_SERVE=1`); micro-VM Cloud Hypervisor path
- SDKs: TypeScript, Python, Go
- Public `GET /status` + opt-in telemetry (`NORIX_TELEMETRY=1`)
- Patroni local stack: `deploy/postgresql/patroni/`
- Release workflow: multi-arch binaries, `SHA256SUMS`, keyless cosign (`.github/workflows/release.yml`)
- SBOM script, k6 smoke, chaos catalog, SECURITY/UPGRADE docs

### Ops notes
- WireGuard live apply: `NORIX_APPLY_WIREGUARD=1`
- Micro-VM boot: KVM + `NORIX_MICROVM_KERNEL` / `NORIX_MICROVM_ROOTFS`
- Publish: create git tag `v1.0.0` to run the Release workflow
