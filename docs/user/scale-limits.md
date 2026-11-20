# Scale limits

Hard caps for a single organization (#45). Defaults match beta.6 supported scale; raise them with env vars on the control plane.

| Resource | Default | Env override |
|---|---:|---|
| Nodes per organization | 32 | `NORIX_MAX_NODES_PER_ORG` |
| Projects per organization | 100 | `NORIX_MAX_PROJECTS_PER_ORG` |
| Workloads per project | 100 | `NORIX_MAX_WORKLOADS_PER_PROJECT` |
| Replicas per workload | 32 | `NORIX_MAX_REPLICAS` |
| Autoscaler max replicas | 8 | `NORIX_MAX_AUTOSCALER_REPLICAS` (also capped by `NORIX_MAX_REPLICAS`; see `NORIX_AUTOSCALER_MAX`) |

Creating past a cap returns an invalid-argument error that names the env var. Restart the control plane after changing these variables.

## What is tested

- Unit tests for `limits.FromEnv` / `CheckReplicas` / autoscaler clamp.
- API rejects `replica_count` above the cap on create/update.
- Enroll rejects a new node when the org is at `NORIX_MAX_NODES_PER_ORG`.

Multi-node soak and higher numbers belong to Stage 2 E2E (#37). Footprint of the binaries: [footprint.md](./footprint.md).
