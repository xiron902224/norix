# Databases

**Databases** manages **app database instances** (Postgres, MySQL, Redis) that Norix runs as containers on a worker node.

## Create

1. Open **Databases** → **New database**
2. Pick project + engine + name
3. Status starts as `provisioning` until the agent creates the container
4. When `ready`, copy the **connection URI** (password shown as `******` in the UI)

Credentials are encrypted at rest (`NORIX_BUILD_SECRETS_KEY`). Data lives in a **named Docker volume** on the assigned node (`norix-mdb-<id>-data`) — not replicated across nodes.

## Engines

| Engine | Image | Port |
|---|---|---|
| PostgreSQL | `postgres:16-alpine` | published host port → 5432 |
| MySQL | `mysql:8.4` | → 3306 |
| Redis | `redis:7-alpine` | → 6379 |

MongoDB catalog entry exists but is **not** provisioned yet.

## Backups

Use **Backup** on a ready instance. The control plane runs `docker exec` dump tools against the managed container (`pg_dump` / `mysqldump` / Redis SAVE). Artifacts land under `NORIX_BACKUP_DIR` (or the volume backups path) and can upload to S3 when configured.

## Ops tips

1. Prefer one writable replica node; drain warns for stateful app volumes — managed DBs are similarly node-local.
2. Put the connection URI into workload env vars, then redeploy the app.
3. Control-plane Postgres (install docs) is separate from these app databases.
