<h1 align="center">Norix</h1>

<p align="center">
  Open-source, self-hostable PaaS control plane.<br>
  Point it at a Git repo or container image — it builds, schedules, routes, and TLS-terminates your app.<br>
  Drive it from the web dashboard; agents + edge on your own VPS.
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue" alt="License" /></a>
  <a href="https://github.com/xiron902224/norix"><img src="https://img.shields.io/badge/github-xiron902224%2Fnorix-0b7285" alt="GitHub" /></a>
  <a href="docs/ESTADO_Y_PLAN_V1.md"><img src="https://img.shields.io/badge/status-beta-0b7285" alt="Status" /></a>
  <a href="VERSION"><img src="https://img.shields.io/badge/version-1.0.0--beta.1-0b7285" alt="Version" /></a>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> ·
  <a href="#how-it-works">How It Works</a> ·
  <a href="#interfaces">Interfaces</a> ·
  <a href="#features">Features</a> ·
  <a href="CONTRIBUTING.md">Contributing</a>
</p>

---

## Quick Start

There's one decision first: **how you run Norix itself** (the control plane). Apps then land on enrolled agents (or the embedded runtime on the same box).

| If you're… | Run Norix as | Where your apps run |
|---|---|---|
| **A VPS / team / always-on** | **Self-hosted server** (`get-norix.sh`) | On that box (Docker) — add more nodes with the agent |
| **Solo / learning on a laptop** | **Local stack** (Postgres + `go run` + Vite) | Embedded local runtime on that machine |
| **Multi-node fleet** | Same control plane + agents | Scheduler places replicas across online workers |

> [!TIP]
> **Self-host on a fresh Linux VPS.** One command installs Postgres + control plane (UI embedded) and mounts the host Docker socket so the first server is both panel and first worker — same shape as Coolify / OpenShip Compose mode.

### Self-hosted server (production)

Requires a Linux host (amd64/arm64), root/sudo, and outbound HTTPS. Until `get.norix.dev` exists, the installer is served from **this GitHub repo** (raw.githubusercontent.com).

```bash
# Latest develop (integration tip):
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/develop/scripts/get-norix.sh | bash

# Pin the Beta 1 release tag:
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/v1.0.0-beta.1/scripts/get-norix.sh \
  | NORIX_REF=v1.0.0-beta.1 bash
```

Override the git ref anytime with `NORIX_REF=` (branch or tag). The script clones into `/data/norix/source`.

What it does (Coolify-shaped):

1. Installs Docker Engine + Compose if missing  
2. Clones this repo into `/data/norix/source`  
3. Writes `/data/norix/source/.env` (random DB passwords + secrets)  
4. `docker compose -f deploy/norix/docker-compose.prod.yml up -d --build`  
5. Prints `http://YOUR_IP:8000`

Open the URL → create the first admin → deploy `nginx:alpine` to verify the local Docker runtime.

```bash
# useful afterwards
curl -sf http://127.0.0.1:8000/healthz
docker compose --env-file /data/norix/source/.env \
  -f /data/norix/source/deploy/norix/docker-compose.prod.yml ps
```

**Add worker nodes** (2nd / 3rd VPS): create a join token under **Nodes**, then on each worker:

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/develop/scripts/install-agent.sh -o install-agent.sh
chmod +x install-agent.sh
# with a release binary, or from a clone:
./install-agent.sh --control-plane https://norix.example.com --token '…' --from-source --no-start
# enable deploy/systemd/norix-agent.service — see docs/runbooks/BETA1_3NODE_INSTALL.md
```

<details>
<summary>Self-host with raw Docker Compose (no curl installer)</summary>

```bash
git clone https://github.com/xiron902224/norix.git && cd norix
git checkout develop
cp deploy/norix/env.production.example .env   # then set strong passwords
# point DATABASE_URL / MIGRATION_* at service hostname `postgres`
docker compose --env-file .env -f deploy/norix/docker-compose.prod.yml up -d --build
```

Panel: `http://SERVER_IP:8000`. The `control-plane` container mounts `/var/run/docker.sock` (host-privileged through the socket — use a trusted host).

**Upgrade:** `git pull` (or re-run `get-norix.sh`) then `docker compose … up -d --build`. Back up `/data/norix/source/.env` off-box.

</details>

### Local stack (development)

```bash
git clone https://github.com/xiron902224/norix.git && cd norix
cp .env.example .env
cp control-plane/.env.example control-plane/.env
cp web/.env.example web/.env

docker compose --env-file .env -f deploy/norix/docker-compose.yml up -d postgres

cd control-plane && set -a && . ./.env && set +a && go run ./cmd/server
# other terminal:
cd web && npm install && npm run dev -- --host 127.0.0.1 --port 5173
```

| Service | URL |
|---|---|
| Dashboard (Vite) | http://127.0.0.1:5173 |
| Control plane | http://127.0.0.1:8080/healthz |
| Self-host panel | http://127.0.0.1:8000 (after `get-norix.sh`) |

**Single binary (embedded UI):** `make build && ./control-plane/bin/norix-control-plane`

---

## How It Works

Point Norix at a source — a **container image** or a **Git repo** — and it runs one loop end to end:

1. **Declare.** Create a project/workload in the dashboard (image, port, replicas, schedule strategy, optional git URL).
2. **Build (git).** An online node clones the repo, auto-detects a `Dockerfile` / `Containerfile` or `compose.yaml`, builds an image, and optionally pushes to a registry.
3. **Schedule.** The control plane places replicas across online, non-cordoned workers (spread / binpack, zones, KVM filter).
4. **Reconcile.** Agents (or the embedded local runtime) apply desired `run` / `stop` until observed state matches.
5. **Route + TLS.** Domains map through the Caddy edge; ACME for HTTPS when configured.
6. **Operate.** Heartbeats, metrics, backups, and alerts under Ops. Cordon/drain evacuates workloads before reschedule.

Deleting a workload marks it **deleting**, stops the container on the owning node, then purges the row — so Docker containers are not left behind.

---

## Interfaces

- **Web dashboard** — projects, workloads, nodes (cordon/drain/zones), domains, ops.
- **Connect RPC / protobuf API** — `proto/v1` shared by Go, TypeScript, and the Rust agent.
- **Rust agent** — enroll, heartbeat, build, reconcile, edge/mesh apply.
- **Embedded local runtime** — same reconcile path inside the control plane when `docker.sock` is available (default self-host).

---

## Features

| | |
|---|---|
| **Git → image → deploy** | Dockerfile / Containerfile auto-detect, Compose build fallback, build logs in UI |
| **Image deploys** | Public images (e.g. `nginx:alpine`) with host port allocation |
| **Multi-replica scheduler** | Spread / binpack, zones, affinity hooks, require-KVM for micro-VMs |
| **Cordon / drain** | Block scheduling; ordered evacuate + reschedule orphans |
| **Domains & TLS** | Caddy edge + ACME path for HTTPS hostnames |
| **Ops** | Postgres backups, node metrics, `node.down` / `deploy.failed` alerts |
| **Tenancy** | Postgres RLS, sessions, RBAC (owner/admin/member) |
| **Identity** | OIDC + TOTP (optional) |
| **SDKs** | TypeScript, Python, Go |

---

## Deploy Anywhere

- **Any VPS** — Hetzner, DigitalOcean, Linode, OVH, homelab
- **Dedicated / bare metal** — as long as Docker runs
- **Multi-server** — one control plane, agents on each worker ([3-node runbook](docs/runbooks/BETA1_3NODE_INSTALL.md))

Same dashboard regardless of where you deploy.

---

## Status

**Beta 1** (`1.0.0-beta.1`) — self-host installer + Compose prod stack are the supported production path. Not GA yet (CI/release assets hardening ongoing).

Living status: [`docs/ESTADO_Y_PLAN_V1.md`](docs/ESTADO_Y_PLAN_V1.md) · Self-host design: [`docs/proposals/SELFHOST_INSTALL.md`](docs/proposals/SELFHOST_INSTALL.md)

---

## Repository layout

| Path | Role |
|---|---|
| `control-plane/` | Go Connect server, migrations, scheduler, local runtime |
| `web/` | React + Vite dashboard (embedded in prod image) |
| `agent/` | Rust node agent |
| `deploy/norix/docker-compose.prod.yml` | Self-host stack |
| `scripts/get-norix.sh` | One-line installer |
| `docs/` | Plans, runbooks, proposals |

---

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md). PRs target **`develop`**.

```bash
cd control-plane && go test ./...
cd web && npm run lint && npm run build
cd agent && cargo test
```

---

## Security

Report vulnerabilities **privately** to the maintainers — not in public issues. Policy: [`SECURITY.md`](SECURITY.md).

The self-host control plane mounts the host Docker socket — treat the VPS as trusted.

---

## License

Licensed under the [Apache License 2.0](LICENSE). Third-party components retain their own licenses.
