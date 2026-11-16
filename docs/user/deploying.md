# Deploying

## Prerequisites

- At least one **online** node with `dockerSocket` (or containerd) **true**
- Workload created under a project

If no online node has Docker or containerd, deploys fail with `no online node with Docker or containerd`. Fix: install Docker on a worker and ensure the agent reports it (see [Nodes](./nodes-agents.md) and runbook note in [`BARE_INSTALL.md`](../runbooks/BARE_INSTALL.md)).

## Deploy from image

1. Open the workload
2. Set image (example: `nginx:alpine`)
3. **Deploy**
4. Wait until status is running; check logs if needed

## Deploy from Git

1. Provide Git URL (+ ref/branch)
2. Dockerfile path if not default
3. **Deploy from Git** — a build runs on a capable node, then the resulting image is scheduled

Build nodes need Docker. Progress and logs appear under builds / workload logs in the UI.

## Compose stacks

Where supported, analyze/deploy Compose from Git creates multiple services under a stack. Prefer the UI flow under the project/workload for Compose when available.

## After deploy

- Hit the service via node IP + published port, or
- Attach a [domain](./domains.md) for HTTPS
