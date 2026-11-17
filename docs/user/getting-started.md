# Getting started

Assumes Norix is already installed and you can open the dashboard URL (for example `http://YOUR_IP:8000` or `https://norix.example.com`).

## 1. First admin

On a fresh install the UI asks you to create the **first admin** (bootstrap). That user owns the first organization.

Bootstrap only works over **HTTPS** (trusted reverse proxy) or from a **loopback** client (`http://127.0.0.1:…`). From a public HTTP URL the API returns a clear error unless `NORIX_ALLOW_INSECURE_BOOTSTRAP=1` (labs / Docker one-click only — remove after TLS).

Secure options:

```bash
# SSH tunnel, then open http://127.0.0.1:8000
ssh -L 8000:127.0.0.1:8000 user@YOUR_HOST
```

Or put TLS in front of the panel — [Production TLS](./production-tls.md).

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
