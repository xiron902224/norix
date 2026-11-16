# Getting started

Assumes Norix is already installed and you can open the dashboard URL (for example `http://YOUR_IP:8000` or `https://norix.example.com`).

## 1. First admin

On a fresh install the UI asks you to create the **first admin** (bootstrap). That user owns the first organization.

If you already have an account: **Login** with email/password (or SSO if your admin enabled OIDC — see [`AUTH_OIDC_TOTP.md`](../AUTH_OIDC_TOTP.md)).

## 2. Check the control plane

- Sidebar: version badge under the **N** logo (from `/status`)
- Or: `curl -sf https://YOUR_CP/status` → `"status":"operational"`

## 3. Add a worker (if you need containers)

Git builds and image deploys need at least one node with **Docker** (or containerd) and an online **agent**.

1. Open **Regions & nodes**
2. **Create join token**
3. On the worker host run the install command shown in the UI (or see [Nodes & agents](./nodes-agents.md))
4. Wait until the node is **online** and **Docker = yes**

Without a schedulable runtime, Deploy / DeployFromGit stays pending or returns unschedulable.

## 4. Create a project and deploy

1. **Projects** → create a project
2. Add a workload (application)
3. Deploy with an image (e.g. `nginx:alpine`) or **Deploy from Git**
4. Open the assigned URL / port, or attach a [domain](./domains.md)

Details: [Projects & workloads](./projects-workloads.md) · [Deploying](./deploying.md)

## 5. Next steps

- TLS hostname → [Domains](./domains.md)
- Managed DB → [Databases](./databases.md)
- Backups → [Ops](./ops.md)
- Keep agents in sync → [Update](../UPDATE_AND_UNINSTALL.md)
