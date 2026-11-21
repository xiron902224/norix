# Multi-node E2E smoke (#37)

Pragmatic CI stack: **Postgres + control plane + 3 agents** on one Docker host.

## Limitation

All agents mount the **same host `docker.sock`**. This proves enroll, heartbeats, cordon/drain, and offline-after-kill. It does **not** prove true multi-host scheduling or per-node Docker isolation (that would need DinD / VMs).

## Quick start

```bash
# From repo root (builds binaries on first run)
./test/multinode/smoke.sh
```

Optional:

| Env | Effect |
|---|---|
| `NORIX_MN_KEEP=1` | Leave compose up after the smoke |
| `NORIX_CP_URL` | Override API base (default `http://127.0.0.1:18080`) |
| `SKIP_MULTINODE_E2E=1` | CI job no-op |

## What the smoke asserts

1. `/healthz` on the control plane  
2. First-admin bootstrap (or login) via Connect JSON `POST /api/v1.AuthService/…` (`test/lib/rpc.sh`)  
3. Three join tokens → three agents → **3 online** nodes  
4. Project + workload with `replicaCount=2`  
5. Deploy (best-effort; `failed_precondition` tolerated when shared-socket runtime is awkward)  
6. Cordon + drain on one node  
7. `docker kill` one agent → status **offline** within ~2 min; ≥2 nodes remain online  
8. Tear down  

## Compose only

```bash
cd test/multinode
cp .env.example .env   # fill NORIX_BUILD_SECRETS_KEY + binary paths
# After bootstrap + join tokens in .env:
docker compose --env-file .env up -d --build
```
