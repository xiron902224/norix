# Projects & workloads

## Projects

A **project** groups related apps (workloads) under your organization.

1. Open **Projects**
2. Create a project (name)
3. Open it to manage workloads

## Workloads (applications)

A **workload** is a deployable unit: container image, Git-built image, Compose service, or other runtime.

Typical fields:

- name
- image (for image deploys)
- env vars
- CPU / memory limits
- ports

From the workload view you can **Deploy**, view logs, assign domains, and trigger **Deploy from Git** when configured.

### Environment variables (dashboard)

1. **Create** — Projects → **New application** (image or git) → **Environment variables** → **Add** → key/value (e.g. `FOO` / `secret`) → Create.
2. **View** — Application → **Summary** lists keys with values masked as `********` (never plaintext).
3. **Edit** — **Edit** tab → change a value or leave `********` to keep the current secret → **Save changes**. Remove a row to delete that key.

Values are stored encrypted at rest; the API merge treats `********` as “unchanged”. Full secret UX (`.env` import, audited reveal) is tracked in [#23](https://github.com/xiron902224/norix/issues/23).

## Lifecycle tips

- Prefer small, reviewable env changes; avoid putting secrets in Git (use build secrets / env in the UI when available).
- Workload **env vars are encrypted at rest** (AES-GCM, `NORIX_BUILD_SECRETS_KEY`). API/UI responses mask values as `********`. Agents still receive plaintext when applying containers. See [S7 operations](../runbooks/S7_OPERATIONS.md) for key rotation.
- Before removing a node, **drain** it so replicas move (Nodes).
- Deleting a workload removes its desired state; agents stop/reconcile containers on the next heartbeat.
