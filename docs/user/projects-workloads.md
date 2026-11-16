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

## Lifecycle tips

- Prefer small, reviewable env changes; avoid putting secrets in Git (use build secrets / env in the UI when available).
- Before removing a node, **drain** it so replicas move (Nodes).
- Deleting a workload removes its desired state; agents stop/reconcile containers on the next heartbeat.
