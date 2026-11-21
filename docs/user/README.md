# Norix user guide

Official documentation for **people who run apps on Norix** — after the control plane is installed.

| | |
|---|---|
| **Install the platform** (operators) | [`README.md`](../../README.md) · [`runbooks/BARE_INSTALL.md`](../runbooks/BARE_INSTALL.md) |
| **Use the dashboard** (this guide) | Start → [Getting started](./getting-started.md) |
| **Español** | [`es/README.md`](./es/README.md) |
| **Upgrade / uninstall** | [`UPDATE_AND_UNINSTALL.md`](../UPDATE_AND_UNINSTALL.md) |
| **Contribute code** | [`CONTRIBUTING.md`](../../CONTRIBUTING.md) |

## Contents

1. [Getting started](./getting-started.md) — first login, org, project
2. [Dashboard overview](./dashboard.md) — navigation map
3. [Projects & workloads](./projects-workloads.md)
4. [Deploying](./deploying.md) — image, Git, Compose
4a. [Compose stacks](./compose-stacks.md) — whole compose file as one unit via `docker compose`
4b. [Release history & rollback](./releases-rollback.md) — immutable snapshots, one-click restore
4c. [Rolling updates](./rolling-updates.md) — start-before-stop, maxSurge / maxUnavailable
4d. [Probes](./probes.md) — liveness / readiness HTTP·TCP·exec
4e. [Persistent volumes](./volumes.md) — named Docker mounts, sticky node, backup/delete
5. [Nodes & agents](./nodes-agents.md) — workers, join tokens, updates
5a. [Faster reconcile](./faster-reconcile.md) — WatchDesired long-poll vs heartbeat (#44)
5b. [Docker socket](./docker-socket.md) — who mounts `docker.sock`, proxy/rootless
5c. [Agent authentication](./agent-auth.md) — rotate/revoke node token, optional mTLS
5d. [CLI (`norix`)](./cli.md) — Rust CLI: deploy, logs, nodes, rollback
6. [Domains & TLS](./domains.md)
7. [Production TLS](./production-tls.md) — domain → DNS → certs → HTTPS panel + apps
8. [Databases](./databases.md)
9. [Network & edge](./network.md)
9b. [Preview environments](./previews.md) — PR → unique URL (#21)
10. [Ops & backups](./ops.md)
10b. [Scale limits](./scale-limits.md) — configurable hard caps (#45)
10c. [Resource footprint](./footprint.md) — binary size / RSS (#43)
11. [Access & teams](./access.md)
12. [Troubleshooting](./troubleshooting.md)
13. [Load tests (k6)](./load-tests.md) — smoke + full API scenarios

## What Norix is

Norix is a **self-hosted PaaS control plane**. You (or your admin) install it on your own VPS. From the web UI you:

- create **projects** and **workloads** (apps)
- deploy from a **container image** or **Git repo**
- attach **domains** with TLS
- run **agents** on one or more machines that execute containers
- manage **databases**, **sandboxes**, **previews**, and **ops** (backups / alerts)

Beta status: see [`ESTADO_Y_PLAN_V1.md`](../ESTADO_Y_PLAN_V1.md). Version pin today: **1.0.0-beta.5** (check the badge under the logo in the UI, or `GET /status`).
