# Update & uninstall

Operator runbook for in-place upgrades and clean removal. Tracks [#78](https://github.com/xiron902224/norix/issues/78) / [#73](https://github.com/xiron902224/norix/issues/73).

Target release examples use **`v1.0.0-beta.5`** — pin whatever tag you want.

## Version truth

| Signal | Source |
|---|---|
| Control plane | `GET /status` → `version` (also sidebar badge) |
| Agent | Heartbeat `capabilities.agentVersion` (Nodes UI) |
| Install pin | `install-state.json` / `NORIX_VERSION` / `NORIX_AGENT_VERSION` |

## Update plan (lab or VPS)

```text
1. Backup Postgres
2. Update control plane → verify /status
3. From UI (or API): Update agent on each worker  OR  run norix-agent update on each node
4. Confirm Nodes shows the same agentVersion everywhere
5. Smoke: deploy a workload
```

### 1. Backup

```bash
# bare lab / VPS
sudo -u postgres pg_dump norix > "/var/lib/norix/backups/norix-$(date +%F).sql"
# or Ops UI backup if enabled
```

### 2. Control plane

**From the binary** (same shape as the agent):

```bash
norix-control-plane check
norix-control-plane update --version v1.0.0-beta.5
norix-control-plane uninstall [--purge] [--purge-db]
```

**Bare** (idempotent re-install / update via installer):

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/v1.0.0-beta.5/scripts/get-norix.sh \
  | NORIX_REF=v1.0.0-beta.5 bash -s -- --mode bare --supervise none   # lab
# VPS with systemd: omit --supervise none (auto → systemd)
```

**Docker compose:**

```bash
./scripts/install.sh upgrade --version v1.0.0-beta.5
```

Verify:

```bash
norix-control-plane check
curl -sf http://127.0.0.1:8000/status   # or your public URL
# → "version": "1.0.0-beta.5"
```

### 3. Agents — same path from CP or the node

**A. From the control-plane UI (recommended for fleets)**  
Nodes → select worker → **Update agent** (sets `desired_agent_version`; agent applies on the next heartbeat).

**B. On the node (local):**

```bash
# Prefer reusing existing agent.env / node-state (no new join token):
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/v1.0.0-beta.5/scripts/install-agent.sh \
  | bash -s -- --update --version v1.0.0-beta.5 --supervise auto

# Or binary subcommands (no enroll required for check):
norix-agent check
norix-agent update --version v1.0.0-beta.5
norix-agent uninstall [--purge]
```

Both update paths: download release → SHA256SUMS (+ cosign when `.cosign.bundle` exists) → replace binary → set `NORIX_*_VERSION` → restart (systemd or nohup).

### 4. Verify

- Nodes: all online, `agentVersion` matches CP (no leftover `desired_agent_version`).
- Deploy a test workload; `dockerSocket` must be true on workers.

## Uninstall

### Agent

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/main/scripts/uninstall-agent.sh | bash
# or: norix-agent uninstall
# purge state:  … | bash -s -- --purge
# also kill labeled containers: … | bash -s -- --purge --purge-workloads
```

### Control plane

```bash
# bare
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/main/scripts/uninstall-norix.sh \
  | bash -s -- --mode bare
# purge data dir + env:  … --purge
# also drop local Postgres DB/roles: … --purge --purge-db

# docker compose
./scripts/install.sh uninstall          # keeps volumes
NORIX_PURGE=1 ./scripts/install.sh uninstall
```

## Rollback

1. Restore DB dump.
2. Re-run CP installer pinned to the previous tag.
3. `install-agent.sh --update --version <old-tag>` on each worker.
