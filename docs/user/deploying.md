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

### Railpack (no Dockerfile)

If the repo has **no** Dockerfile / Containerfile / compose file, Norix falls back to **[Railpack](https://railpack.com/)** when it finds:

| Marker | Detected as |
|---|---|
| `package.json` | Node.js |
| `go.mod` | Go |
| `pyproject.toml` / `requirements.txt` / `Pipfile` | Python |
| `index.html` | Static site |

The builder node needs `railpack` on `PATH` and a BuildKit daemon (`BUILDKIT_HOST`, or Norix starts a `norix-buildkit` container). Install:

```bash
curl -sSL https://railpack.com/install.sh | sh
```

Examples without Dockerfiles: [`examples/`](../../examples/). Build logs stream in the dashboard (Build logs tab) the same as Dockerfile builds.

Build nodes need Docker. Progress and logs appear under builds / workload logs in the UI.

### Git URL SSRF controls

Norix validates Git remotes before clone (control plane and agent):

- Allowed schemes by default: `https://`, `ssh://`, and scp-like `git@host:path`
- `http://`, `file://`, and `git://` are rejected
- After DNS lookup, loopback, RFC1918, link-local, CGNAT (`100.64.0.0/10`), and IPv6 ULA are rejected (including cloud metadata `169.254.169.254`)

For private Git hosts (lab / self-hosted), set a comma-separated allowlist on the control plane **and** agent:

```bash
# hostnames and/or CIDRs that may resolve to otherwise-blocked addresses
export NORIX_GIT_SSRF_ALLOWLIST=git.internal.example,10.10.0.0/16
```

Allowlisting a hostname still does **not** reopen blocked schemes (`http://`, etc.).

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
