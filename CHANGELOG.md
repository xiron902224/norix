# Changelog

## Unreleased

### Added
- Per-project deployment notifications (#27): webhook, Slack, Discord, and email (SMTP). Channels store encrypted targets, subscribe to `deploy.succeeded` / `deploy.failed` / `node.down`, and deliver through a transactional outbox with retries. Webhook URLs reuse the Git SSRF denylist (`NORIX_GIT_SSRF_ALLOWLIST`). Email needs `NORIX_SMTP_ADDR` and `NORIX_SMTP_FROM`. UI: Project → Notifications.
- Compose stacks share a project network (`norix-p-<project id>`). Every service gets the aliases `<service>` and `<compose project>-<service>`, and `external: true` networks keep their aliases (#144).
- Compose stack variables: set `NAME=value` in the form or in Settings. Values are write-only and stored encrypted, and `${VAR}`, `${VAR:-default}` and `${VAR:?}` interpolate through `--env-file`. `SERVICE_PASSWORD_*` and `SERVICE_BASE64_*` secrets are generated once and reused (#145).
- Compose stacks: services are internal by default (host ports removed with `!reset`). **Public services** keeps them. **Domains** route `service=hostname` through Caddy with HTTPS/WSS (#146).
- Compose stacks: the card lists named and bind volumes. Delete keeps volumes by default; **Purge volumes on delete** runs `down --volumes` and removes the checkout (#147).

- Example apps: `examples/compose-multi-service` (db, api, web) is a Compose stack example. CI starts it with `docker compose up --wait` and checks that api reaches db over the service alias (#35).

- `norix doctor` (Rust CLI) and `scripts/install.sh preflight`: read-only checks for Docker, Compose v2, overlay, ports, disk, RAM, KVM, DNS (`--domain`). Each non-pass result prints a fix. `up` runs the preflight first; `NORIX_SKIP_PREFLIGHT=1` skips it (#34).

- Parallel builds per node (`NORIX_BUILD_CONCURRENCY`, default 2). Builds over the limit show `queued` in the Build tab. `NORIX_BUILD_TIMEOUT_SECS` caps each build and kills its children. `NORIX_BUILDKIT_CPUS`/`NORIX_BUILDKIT_MEMORY` limit the shared BuildKit container. Concurrent Railpack builds no longer race to start BuildKit (#24).

### Changed
- Compose stacks: `norix.override.json` is replaced by `norix.override.yml` (generated YAML). Bind mounts must stay inside the checkout or in `NORIX_COMPOSE_BIND_ALLOWLIST`. A failed `git fetch` keeps the checkout instead of deleting it (#147).

### Added
- Compose stacks run as one unit: the agent clones the repo and runs `docker compose -p norix-<id> up -d --build --remove-orphans` in the background (heartbeats keep flowing). It reports per-service state, health and exit code, plus per-service logs. Start/Stop/Redeploy/Delete act on the whole stack, and Delete keeps named volumes. New `compose_stacks` / `compose_stack_logs` tables with forced RLS (migration 000033). `NORIX_COMPOSE_UP_TIMEOUT_SECS` sets the agent timeout. The old per-service split remains as "legacy" (#143).
- Log retention by age, line count, and bytes (`NORIX_LOG_RETENTION_DAYS`, `NORIX_*_MAX_LINES`, `NORIX_*_RETENTION_BYTES`). The leader prunes `workload_logs`, `build_logs`, and `log_segments` (#25).
- Workload **Stop** and **Restart** on the application page. Stop keeps the row and asks the agent to remove the container; Restart redeploys (new generation). Pause is not a runtime state.
- Rust CLI (`norix`): deploy, logs -f, env, domains, nodes, releases, rollback, completions, `--output json`; ADR-005; replaces Go stub (#28)
- Managed databases actually provision Postgres/MySQL/Redis containers via the agent (named volume, connection URI, docker-exec backups) (#20)
- Git builds without Dockerfile fall back to Railpack (Node/Go/Python/static markers); examples + CI; docs EN/ES (#18)
- Persistent named volumes for workloads: declare mounts, sticky node placement, drain warnings, Backup/Delete API + UI, docs EN/ES (#16)

### Fixed
- A git build no longer blocks the agent heartbeat. Deletes, compose actions and image deploys keep flowing while `docker build` runs, and a workload stays `building` (Build logs tab) until the image exists.
- Compose stack display names may include spaces, dots, and letters outside ASCII (`Infra de abogados`). The Docker Compose project name still comes from the stack id.
- `ReportWasm` is authenticated with the node token like the other agent reports. Before, the control plane treated it as a user call and rejected it.
- Backup restore no longer fails with `relation "alert_events" already exists`. The dump uses `--clean --if-exists`, and restore drops schema `public` before loading the file.
- The inventory node `control-plane` is not advertised as a Docker worker unless embedded runtime is on, so deploys are not stuck on a node that never runs containers.
- First-admin bootstrap and the login rate limit read the client IP from Connect v2 `CallInfo.PeerAddr`, so loopback HTTP is accepted again (#6)
- Agent node tokens can be rotated (heartbeat delivery) and revoked from UI/API; optional mTLS fingerprint (#13)
- Git SSRF: validate clone URLs (https/ssh only; block private/metadata ranges; `NORIX_GIT_SSRF_ALLOWLIST`) (#4)
- Control plane no longer panics on legacy AES-GCM `dev-nonce` network secrets
- First-admin bootstrap blocked on plain HTTP from non-loopback unless `NORIX_ALLOW_INSECURE_BOOTSTRAP=1` (#6)
- Workload `env_vars` encrypted at rest (AES-GCM); API masks values; startup backfill for legacy plaintext (#8)
- Docker Compose no longer mounts `docker.sock` into the control plane; agent holds the socket (#10)

### Added
- Personal access tokens (`norix_pat_…`): Create/List/Revoke API + Teams UI; Bearer auth after session miss; hashed at rest (#22)
- Env UX: bulk `.env` import in the editor + audited `RevealWorkloadEnv` (owner/admin) (#23)
- Installer `--panel-domain` / `--acme-email` wires Caddy in front of the panel with ACME HTTP-01 (`docker-compose.panel-tls.yml`); Pebble overlay + smoke script (#26)
- Workload liveness/readiness probes (HTTP/TCP/exec); unready removes edge routing; liveness restarts container; UI probe JSON + Ready status (#17)
- Docs EN/ES: probes.md (#17)
- Rolling updates: start-before-stop with surge host port + readiness HTTP/TCP; workload knobs `update_strategy` / `max_surge` / `max_unavailable`; edge refresh on host_port change (#15)
- Docs EN/ES: rolling-updates.md (#15)
- Immutable workload release history + one-click rollback (API/UI); retention `NORIX_RELEASE_RETENTION`; optional `NORIX_AUTO_ROLLBACK` on failed apply (#14)
- Docs EN/ES: releases-rollback.md (#14)
- Installer/agent verify cosign bundles when present on GitHub Releases; README EN/ES download → verify → run (#5)
- Offline smoke `scripts/test-release-verify.sh` for SHA256 fail-closed behavior (#5)
- Installer prints SSH-tunnel / TLS next steps for secure bootstrap (#6)
- Dashboard env vars editor on create/edit workloads; Summary shows masked `********` (#8, partial #23)
- `NORIX_BOOTSTRAP_JOIN_TOKEN` seed + Compose agent by default; opt-in `docker-compose.embedded-runtime.yml` (#10)
- Docs EN/ES: Docker socket threat model, proxy/rootless notes (#10)

### Changed
- Docs: bare/agent integrity notes (EN + ES), `SECURITY.md` installer section (#5)
- Docs EN/ES: getting-started + production-tls bootstrap rules (#6)
- Docker one-click writes `NORIX_ALLOW_INSECURE_BOOTSTRAP=1` (remove after TLS) (#6)
- Docs EN/ES: workload env encryption + key rotation (projects-workloads, S7 ops, ADR-003, SECURITY) (#8)
- Docs EN/ES: dashboard env editor steps in projects-workloads (#8)
- Docs EN/ES: docker-socket.md; embedded runtime gated by `NORIX_EMBEDDED_RUNTIME` (#10)
- ADR-004 + docs EN/ES agent auth (rotate/revoke/mTLS) (#13)

## 1.0.0-beta.5 — 2026-11-16

### Fixed
- DeployFromGit maps unschedulable / placement errors to `FailedPrecondition` (#59)
- Distinct scheduler error when no online node has Docker/containerd (#60)
- Control plane fails closed without `NORIX_BUILD_SECRETS_KEY` (opt-in `NORIX_ALLOW_INSECURE_DEV_SECRETS`) (#7)
- Installer CORS defaults to panel origin; wildcard needs `NORIX_CORS_ALLOW_WILDCARD=1` (#9)
- GitHub deploy webhooks require `X-Hub-Signature-256` (#3)
- Preview webhook requires `NORIX_GITHUB_WEBHOOK_SECRET` (#11)

### Added
- `install-agent` warning when Docker/containerd sockets are missing (#63)
- CI: `govulncheck` / `npm audit` / `cargo audit` + Dependabot (#12)
- CI upgrade smoke N-1 → N (`scripts/ci-upgrade-smoke.sh`) (#38)
- Dashboard deploy progress stepper + first-run onboarding (#36, #19)
- Production TLS guide + expanded troubleshooting (#31, #32)
- Status/maturity refresh + repo About topics + README preview (#30, #33)

### Changed
- Go CI/release pin → 1.27.4; `lego` → v4.25.2 (#12)
- Required branch-protection checks documented and aligned (#67)
- Docs / installer pins → `v1.0.0-beta.5`

### Notes
- Stage 0 deeper items (#4–#6, #8, #10, #13…) remain open and do not block this beta
- Tag `v1.0.0-beta.5` to run the Release workflow

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
