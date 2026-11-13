<h1 align="center">Norix</h1>

<p align="center">
  Open-source, self-hostable PaaS control plane.<br>
  Point it at a Git repo or container image — it builds, schedules, routes, and TLS-terminates your app.<br>
  Drive it from the web dashboard today; agent + edge on your own VPS.
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-TBD-lightgrey" alt="License" /></a>
  <a href="https://github.com/xiron902224/norix"><img src="https://img.shields.io/badge/github-xiron902224%2Fnorix-0b7285" alt="GitHub" /></a>
  <a href="docs/ESTADO_Y_PLAN_V1.md"><img src="https://img.shields.io/badge/status-alpha-0b7285" alt="Status" /></a>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> ·
  <a href="#how-it-works">How It Works</a> ·
  <a href="#interfaces">Interfaces</a> ·
  <a href="#features">Features</a> ·
  <a href="docs/ESTADO_Y_PLAN_V1.md">Docs</a>
</p>

---

## Quick Start

There's one decision first: **how you run Norix itself** (the control plane). Apps then land on enrolled agents (or the embedded local runtime on the same box).

| If you're… | Run Norix as | Where your apps run |
|---|---|---|
| **Solo / learning on one machine** | **Local stack** (Postgres + control plane + Vite) | Embedded local runtime (Docker) on that box |
| **A VPS / always-on host** | **Control plane + agent** on the server | Docker/nerdctl on that host (and more nodes later) |
| **Multi-node** | Same control plane | Scheduler places replicas across online workers |

> [!TIP]
> **Local first.** Bring up Postgres + control plane + the dashboard. Deploy `nginx:alpine` before you wire git builds or domains — that validates reconcile end to end in minutes.

### Local stack

```bash
git clone https://github.com/xiron902224/norix.git && cd norix

cp .env.example .env
cp control-plane/.env.example control-plane/.env
cp web/.env.example web/.env

# Postgres (+ optional edge / registry profiles)
docker compose --env-file .env -f deploy/norix/docker-compose.yml up -d postgres

# Control plane (migrates DB on boot)
cd control-plane && set -a && . ./.env && set +a && go run ./cmd/server

# Dashboard (another terminal)
cd web && npm install && npm run dev -- --host 127.0.0.1 --port 5173
```

| Service | URL |
|---|---|
| Dashboard | http://127.0.0.1:5173 |
| Control plane / health | http://127.0.0.1:8080/healthz |
| Postgres | `127.0.0.1:5432` |
| Caddy HTTPS (optional edge profile) | https://127.0.0.1:8443 |

Bootstrap the first org/user in the UI, create a project, add a workload (`nginx:alpine`), and **Deploy**. The embedded local node reconciles containers when Docker is available on the host.

<details>
<summary>Optional: edge (Caddy + Pebble ACME) and registry (zot)</summary>

```bash
docker compose --env-file .env -f deploy/norix/docker-compose.yml up -d postgres pebble caddy
# registry (optional):
docker compose --env-file .env -f deploy/norix/docker-compose.yml --profile registry up -d
```

Domains are managed in the dashboard; TLS for local/dev uses Pebble. See `docs/SPRINT_6_CIERRE.md`.

</details>

<details>
<summary>Optional: remote Rust agent</summary>

```bash
# From the Nodes page: create a join token, then:
cd agent
NORIX_JOIN_TOKEN='…' NORIX_CONTROL_PLANE_URL=http://127.0.0.1:8080 \
  cargo +1.88.0 run --release
```

The agent enrolls, heartbeats, runs git builds, and reconciles desired containers.

</details>

**Single binary (embedded UI):**

```bash
make build
./control-plane/bin/norix-control-plane
# open http://127.0.0.1:8080
```

---

## How It Works

Point Norix at a source — a **container image** or a **Git repo** — and it runs one loop end to end:

1. **Declare.** Create a project/workload in the dashboard (image, port, replicas, schedule strategy, optional git URL).
2. **Build (git).** An online node clones the repo, auto-detects a `Dockerfile` / `Containerfile` or `compose.yaml` / `docker-compose.yml`, builds an image, and optionally pushes to the local registry.
3. **Schedule.** The control plane places replicas across online, non-cordoned workers (spread / binpack, zones, KVM filter).
4. **Reconcile.** Agents (or the embedded local runtime) apply desired `run` / `stop` until observed state matches.
5. **Route + TLS.** Domains map through the Caddy edge; local ACME via Pebble when the edge stack is up.
6. **Operate.** Heartbeats, node metrics, backups, and alerts live under Ops. Cordon/drain evacuates workloads with an ordered stop before reschedule.

Deleting a workload marks it **deleting**, sends `stop` to the node that owns the container, then purges the row once the container is gone — so Docker containers are not left behind.

---

## Interfaces

- **Web dashboard** — projects, workloads, nodes (cordon/drain/zones), domains, ops.
- **Connect RPC / protobuf API** — `proto/v1` contracts shared by Go, TypeScript, and the Rust agent.
- **Rust agent** — enroll, heartbeat, build, reconcile, edge plan apply.
- **Embedded local runtime** — same reconcile/build path inside the control plane when no remote agent is required.

---

## Features

| | |
|---|---|
| **Git → image → deploy** | Dockerfile / Containerfile auto-detect, Compose build fallback, build logs in UI |
| **Image deploys** | Public images (e.g. `nginx:alpine`) with host port allocation |
| **Multi-replica scheduler** | Spread / binpack, zones, affinity hooks, require-KVM for micro-VMs |
| **Cordon / drain** | Block scheduling; ordered evacuate + reschedule orphans (&lt; ~60s target) |
| **Domains & TLS** | Caddy edge + Pebble (dev) / ACME path for HTTPS hostnames |
| **Ops** | pg backups, node metrics, `node.down` / `deploy.failed` alerts |
| **Tenancy** | Postgres RLS, sessions, RBAC (owner/admin/member) |

---

## Repository layout

| Path | Role |
|---|---|
| `control-plane/` | Go Connect server, migrations, scheduler, local runtime |
| `web/` | React + Vite dashboard |
| `agent/` | Rust node agent |
| `proto/` | Shared protobuf contracts |
| `deploy/norix/` | Compose stack (Postgres, Caddy, Pebble, optional zot) |
| `docs/` | Sprint plan and architecture (`docs/ESTADO_Y_PLAN_V1.md`) |

---

## Status

**Alpha v0.1 (S6 reached).** One-node path works: auth, projects/workloads, local runtime and/or Rust agent, git→build, Caddy domains/TLS (Pebble), basic ops.

**Canonical roadmap:** **S0–S22** through GA 1.0 — see [`docs/PLAN_SPRINTS_S0_S22.md`](docs/PLAN_SPRINTS_S0_S22.md).  
**Where we are:** [`docs/ESTADO_Y_PLAN_V1.md`](docs/ESTADO_Y_PLAN_V1.md) — **S16 closed**; next **S17** HA leases → S18–S22 GA.

---

## Contributing

Roadmap: `docs/PLAN_SPRINTS_S0_S22.md`. Living status: `docs/ESTADO_Y_PLAN_V1.md`. Keep secrets out of git (`.env` is ignored). Prefer small, reviewable PRs against `develop`.

```bash
cd control-plane && go test ./...
cd web && npm run build && npm run lint
```

---

## Security

Report vulnerabilities **privately** to the maintainers — not in public issues. Do not commit production credentials or join tokens.

---

## License

License file is not finalized yet (badge TBD). Third-party components retain their own licenses.
