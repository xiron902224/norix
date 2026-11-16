# Deploying

## Prerequisites

- At least one **online** node with `dockerSocket` (or containerd) **true**
- Workload created under a project

If no runtime is available, deploys fail with unschedulable / no feasible node. Fix: install Docker on a worker and ensure the agent reports it (see [Nodes](./nodes-agents.md) and runbook note in [`BARE_INSTALL.md`](../runbooks/BARE_INSTALL.md)).

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

## GitHub push webhook

Mint a token (`POST /api/webhooks/workloads/{workloadID}` with a session Bearer). Response includes `webhook_url` and `secret` (same value as the path token).

In GitHub → repo → **Settings → Webhooks → Add webhook**:

1. **Payload URL** — `https://YOUR_CP/hooks/github/<token>`
2. **Content type** — `application/json`
3. **Secret** — paste the same `<token>` (Norix verifies `X-Hub-Signature-256`)
4. Event — just the `push` event

Requests without a valid signature receive **401**.

## Compose stacks

Where supported, analyze/deploy Compose from Git creates multiple services under a stack. Prefer the UI flow under the project/workload for Compose when available.

## After deploy

- Hit the service via node IP + published port, or
- Attach a [domain](./domains.md) for HTTPS
