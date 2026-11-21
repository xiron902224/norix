# Norix — Vision & Roadmap

> The self-hosted PaaS with a real scheduler and hard multi-tenant isolation.
> Not "Docker on one server with a nice UI" — a control plane that places, isolates and reconciles workloads across many nodes.

Status: **1.0.0-beta.4** · Audit date: 2026-11-15 · License: Apache-2.0
Living docs: this file (vision) · [`ISSUES.md`](ISSUES.md) (GitHub backlog triage) · [`docs/ESTADO_Y_PLAN_V1.md`](docs/ESTADO_Y_PLAN_V1.md) (sprint detail)

---

## 1. What Norix is

Point Norix at a Git repo or a container image. It builds, schedules replicas across your nodes, reconciles desired vs. observed state, routes traffic through a Caddy edge and terminates TLS. You drive it from a web dashboard, a CLI or a Connect/protobuf API.

| Piece | Tech | Role |
|---|---|---|
| Control plane | Go (Connect RPC), Postgres 16 | API, scheduler, auth, migrations, embedded dashboard |
| Node agent | Rust (tokio) | Enroll, heartbeat, build, run workloads, edge/mesh apply |
| Dashboard | React 19 + Vite | Projects, workloads, nodes, domains, ops |
| Edge | Caddy | Routing + ACME |
| Contracts | protobuf (`proto/v1`) | One API shared by Go, TypeScript and Rust |

## 2. Why Norix is different

Most self-hosted PaaS tools are Compose-centric: one server (or SSH to several), containers sharing a kernel, a UI on top. Norix is built around different bets:

1. **A real scheduler, not just "deploy to server X".** Spread / binpack, zones, affinity / anti-affinity, `require-KVM`, cordon / drain and orphan reclaim — in the control plane, with tests.
2. **Multi-tenancy as a database property.** Postgres Row-Level Security (`FORCE`, 38 tables, cross-tenant isolation test in CI), plus RBAC, OIDC and TOTP. Tenancy is enforced below the application code.
3. **Isolation beyond containers (in progress).** Cloud Hypervisor micro-VMs and Wasmtime are wired into the agent so untrusted or multi-tenant workloads don't have to share a kernel. *Experimental today — see §3.*
4. **Tiny footprint.** Rust agent (~4.4 MiB binary) and a single Go control-plane binary (~30 MiB, UI embedded). No Swarm, no Kubernetes.
5. **One typed API for everything.** The dashboard, agent, CLI and SDKs speak the same protobuf contracts, so integrations and automation are first-class, not an afterthought.
6. **Built-in HA and mesh primitives.** Control-plane leases, WireGuard mesh planning, and a Patroni deployment recipe for Postgres.

**Who it's for:** teams and platform builders who need to run many apps (or other people's apps) across several machines they control, with real isolation and placement control — internal developer platforms, hosting providers, sandbox / code-execution services, agencies running client workloads.

**Who it's not for (yet):** someone who wants one-click WordPress on a single VPS today. Coolify and Dokploy are excellent at that.

## 3. Honest status (feature maturity)

We label every capability so nobody has to guess. Source: repository audit of 2026-11-15 (static review + partial test run).

| Maturity | Meaning |
|---|---|
| **Stable** | Implemented, tested, documented |
| **Beta** | Works on the happy path; gaps or limited tests |
| **Experimental** | Behind a flag / partially wired; do not rely on it |
| **Planned** | Not in code yet |

| Capability | Maturity | Notes |
|---|---|---|
| Deploy from image | Stable | Docker / nerdctl runtime via the agent |
| Deploy from Git (Dockerfile / Containerfile) | Beta | Fails when no Dockerfile or compose file is found |
| Compose stack deploy | Beta | Parsed into N workloads |
| Scheduler, cordon / drain, desired / observed reconcile | Stable | Unit tests; multi-node E2E not yet in CI |
| Multi-tenancy (RLS), RBAC, OIDC, TOTP, audit log | Stable | RLS isolation test in CI |
| Dashboard (projects, nodes, domains, ops, teams…) | Beta | One Playwright smoke test |
| Installer (docker / bare), join tokens | Beta | Release **SHA256** verified; agent `--supervise` systemd/nohup |
| Git webhooks → build | Beta | HMAC `X-Hub-Signature-256` required (path token = GitHub Secret; see #3) |
| Domains + Caddy edge | Beta | Route health + rollback of edge upstreams |
| TLS / ACME | Experimental | DNS-01 path exists; control plane itself serves plain HTTP |
| Preview environments | Experimental | DB rows + TTL; deploy pipeline is a stub |
| Managed databases | Beta | Postgres/MySQL/Redis containers + URI (#20); Mongo not yet |
| Autoscaler | GA (flag) | `NORIX_AUTOSCALER=1`; UI/API per-workload policy (#49) |
| WireGuard mesh | Experimental | Dry-run by default |
| Wasm (Wasmtime) | Experimental | `NORIX_WASM_SERVE=1`; OCI → Wasm materialization incomplete |
| Micro-VM (Cloud Hypervisor) | Experimental | Needs KVM + kernel/rootfs artifacts |
| Micro-VM (Firecracker) | Planned | Enum only |
| App rollbacks / release history | Planned | |
| Rolling / zero-downtime updates | Planned | Update is currently stop → start |
| Persistent volumes for apps | Beta | Named Docker mounts, sticky node, backup/delete (#16) |
| Buildpacks / build without a Dockerfile | Beta | Railpack fallback when no Dockerfile (#18) |
| One-click app catalog | Planned | |
| CLI | Experimental | `health`, `login`, `projects`; `deploy` is a stub |
| SDKs (TS / Python / Go) | Experimental | Minimal clients |
| User API tokens (PAT) | Planned | |

## 4. How we compare (Nov 2026)

Competitor data comes from their public docs and third-party reviews (Coolify's own comparison page, Dokploy docs, independent reviews) checked in Nov 2026; verify before quoting. ✅ yes · ◐ partial · ❌ no · — not documented.

| | **Norix** (today) | Coolify | Dokploy | CapRover | Dokku | Kamal |
|---|---|---|---|---|---|---|
| Deploy from Git / image | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ (image) |
| Build without a Dockerfile | ❌ | ✅ Nixpacks / Railpack | ✅ Nixpacks, Railpack, buildpacks | ◐ | ✅ buildpacks | ❌ |
| Rollbacks | ❌ | ✅ | ✅ | ✅ | ◐ | ✅ |
| Preview deployments | ◐ stub | ✅ | ✅ | — | ◐ | — |
| One-click catalog | ❌ | ✅ 360+ | ✅ | ✅ | ❌ | ❌ |
| Multi-node | ✅ agents | ◐ SSH, no scheduling | ✅ Swarm + remote servers | ✅ Swarm | ❌ | ◐ SSH hosts |
| Placement control (spread / binpack / zones / affinity) | ✅ | ❌ | ◐ via Swarm | ◐ via Swarm | ❌ | ❌ |
| Cordon / drain | ✅ | ❌ | ◐ via Swarm | ◐ via Swarm | ❌ | ❌ |
| DB-enforced multi-tenancy (RLS) | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| RBAC / SSO | ✅ RBAC, OIDC, TOTP | ◐ team-wide roles | ◐ custom roles are Enterprise | ◐ 2FA in Pro | ❌ | ❌ |
| Isolation beyond containers | ◐ experimental micro-VM / Wasm | ❌ | ❌ | ❌ | ❌ | ❌ |
| MCP server for AI agents | ❌ | ✅ | — | — | — | — |
| Typed API shared with the agent | ✅ protobuf | REST | REST | REST | CLI | CLI |
| Persistent volumes / backups UX | ❌ apps · ◐ DB backups | ✅ | ✅ | ◐ | ✅ plugins | ◐ accessories |
| Community / maturity | New (beta) | Very large (60K+ stars) | Large | Medium | Very large | Large |

**Takeaway.** Norix will not win by matching Coolify's catalog first. It wins where others are structurally weak: placement, tenancy and isolation. But it must first close the "basics" gap (rollbacks, volumes, buildpacks, HTTPS out of the box), because those decide whether anyone gets far enough to care about the scheduler.

## 5. Roadmap

Stages are ordered by dependency and impact. Each stage has an **exit criterion** — a statement we can check, not a date. Issue numbers are **GitHub issues** (see [`ISSUES.md`](ISSUES.md)).

### Stage 0 — Trust & safety (in progress through beta.3 → before rc.1)
*Goal: nobody is surprised by what Norix does, and the obvious attack paths are closed.*
- Webhook HMAC ([#3](https://github.com/xiron902224/norix/issues/3)), SSRF-safe Git clone ([#4](https://github.com/xiron902224/norix/issues/4)), installer/binary integrity ([#5](https://github.com/xiron902224/norix/issues/5) — SHA256 + cosign when bundle present), TLS-first admin bootstrap ([#6](https://github.com/xiron902224/norix/issues/6) — loopback/HTTPS or opt-in), fail-closed secrets ([#7](https://github.com/xiron902224/norix/issues/7)), encrypted env vars ([#8](https://github.com/xiron902224/norix/issues/8) — AES-GCM + API mask), safe CORS ([#9](https://github.com/xiron902224/norix/issues/9)), Docker-socket threat model ([#10](https://github.com/xiron902224/norix/issues/10)).
- Vulnerability scanning in CI ([#12](https://github.com/xiron902224/norix/issues/12)).
- Truthful docs: maturity table + status refresh ([#30](https://github.com/xiron902224/norix/issues/30), [#33](https://github.com/xiron902224/norix/issues/33)).
- **Exit:** a security review finds no Critical/High open; every feature in the README matches its maturity label. *(Not claimed at beta.3.)*

### Stage 1 — Table stakes (→ 1.0.0-rc.1)
*Goal: a developer can take a normal app from repo to HTTPS in production and recover from a bad deploy.*
- Release history + rollback ([#14](https://github.com/xiron902224/norix/issues/14)), rolling updates ([#15](https://github.com/xiron902224/norix/issues/15)), volumes ([#16](https://github.com/xiron902224/norix/issues/16)), probes ([#17](https://github.com/xiron902224/norix/issues/17)).
- Build without Dockerfile ([#18](https://github.com/xiron902224/norix/issues/18)), real managed DBs ([#20](https://github.com/xiron902224/norix/issues/20)), env/secret UX ([#23](https://github.com/xiron902224/norix/issues/23)).
- HTTPS out of the box ([#26](https://github.com/xiron902224/norix/issues/26)), user API tokens ([#22](https://github.com/xiron902224/norix/issues/22)).
- Real CLI ([#28](https://github.com/xiron902224/norix/issues/28)) and typed SDKs ([#29](https://github.com/xiron902224/norix/issues/29)).
- **Exit:** the 10-minute walkthrough (install → deploy → domain → HTTPS → rollback) works on a fresh VPS with no manual Caddy / ACME assembly.

### Stage 2 — Production confidence (→ 1.0.0 GA)
*Goal: we can prove it works across nodes and across upgrades.*
- Multi-node E2E ([#37](https://github.com/xiron902224/norix/issues/37)), upgrade N-1 → N ([#38](https://github.com/xiron902224/norix/issues/38)), agent coverage ([#39](https://github.com/xiron902224/norix/issues/39)), load/chaos ([#40](https://github.com/xiron902224/norix/issues/40), [#41](https://github.com/xiron902224/norix/issues/41)), Playwright ([#42](https://github.com/xiron902224/norix/issues/42)).
- Footprint / scale limits ([#43](https://github.com/xiron902224/norix/issues/43), [#45](https://github.com/xiron902224/norix/issues/45)), faster reconcile ([#44](https://github.com/xiron902224/norix/issues/44)).
- **Exit:** CI runs a 3-node scenario (deploy, drain, kill agent, upgrade) green; published numbers for memory, nodes and workloads supported.

### Stage 3 — Differentiation (1.x)
*Goal: deliver the things nobody else in self-hosted PaaS has.*
- Micro-VM GA ([#46](https://github.com/xiron902224/norix/issues/46)), Wasm GA ([#47](https://github.com/xiron902224/norix/issues/47)), WireGuard default ([#48](https://github.com/xiron902224/norix/issues/48)), autoscaler GA ([#49](https://github.com/xiron902224/norix/issues/49)).
- Sandboxes API ([#50](https://github.com/xiron902224/norix/issues/50)), Firecracker ([#51](https://github.com/xiron902224/norix/issues/51)), Postgres HA runbook ([#52](https://github.com/xiron902224/norix/issues/52)).
- **Exit:** a multi-tenant demo where two tenants' workloads run in separate micro-VMs on shared nodes, with a documented isolation model.

### Stage 4 — Ecosystem (1.x → 2.0)
- MCP server ([#53](https://github.com/xiron902224/norix/issues/53)), app catalog ([#54](https://github.com/xiron902224/norix/issues/54)), SAML ([#55](https://github.com/xiron902224/norix/issues/55)), quotas ([#56](https://github.com/xiron902224/norix/issues/56)), Terraform ([#57](https://github.com/xiron902224/norix/issues/57)).
- **Exit:** third parties ship templates and integrations without touching core.

## 6. Principles

1. **Label maturity honestly.** A flag-gated feature is "Experimental", never "ready".
2. **Secure by default.** Safe defaults beat convenient ones; convenience goes in an explicit opt-in.
3. **State is reconciled, not scripted.** Desired vs. observed, idempotent, safe to retry.
4. **One contract.** Anything the dashboard can do, the API, CLI and SDKs can do.
5. **Small and boring to operate.** Few moving parts; a single binary plus an agent.

## 7. Non-goals

- Replacing Kubernetes for large, heterogeneous clusters.
- A hosted SaaS offering inside this repository.
- Supporting every language runtime natively — we build images (Dockerfile, Railpack) and run them.

## 8. Contributing

- Pick an issue from `ISSUES.md` (labels: `good first issue`, `help wanted`, area labels such as `area/agent`, `area/control-plane`, `area/web`, `area/security`).
- Read `CONTRIBUTING.md` / `CONTRIBUTING.es.md`: branch from **`main`**, PR to **`main`**.
- Security problems: report privately per `SECURITY.md`.

*We keep this file short on purpose. If a claim here is wrong, open an issue — correcting it is a contribution.*
