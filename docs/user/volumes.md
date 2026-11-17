# Persistent volumes

Workloads can declare **named Docker volumes**. Data lives on the **pinned worker node** (not replicated across the cluster).

## Configure (UI)

Application → **Edit** → **Volumes (JSON)** → Save → Deploy. Example:

```json
[{"name": "data", "mount_path": "/var/lib/app"}]
```

Optional `read_only: true`. Names: `[a-zA-Z0-9._-]`, max 63. Mount paths must be clean absolute paths (not `/`).

On **Summary**, use **Backup** / **Delete** per volume (requires an assigned node). Delete asks for confirmation and removes the Docker volume on the node after the agent applies the op.

## Behaviour

| Topic | Limit |
|---|---|
| Placement | Scheduler **pins** all replicas with volumes to the first placed node |
| Drain | Warns naming stateful workloads; data is **not** auto-moved |
| Orphan reschedule | Stateful replicas are **not** moved when a node disappears |
| Backup | Agent writes a tar under `/var/lib/norix/volume-backups` on that node |
| Delete workload | Does **not** delete Docker volumes — use explicit **Delete** on the volume |
| Rolling updates | Prefer **recreate** when mounts are R/W (surge can share the same volume) |

Docker volume name pattern: `norix-wv-<12hex>-<logical>`.

## Ops tips

1. Prefer **replica_count = 1** for writable state.
2. Back up before drain / node replacement.
3. See also [Rolling updates](./rolling-updates.md) and [Nodes & agents](./nodes-agents.md).
