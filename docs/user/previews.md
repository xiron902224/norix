# Preview environments

PR previews deploy a unique URL for each pull request (#21).

## How it works

1. Configure the control plane with a target org/project and GitHub secrets.
2. Add a GitHub webhook for `pull_request` events to `POST /api/webhooks/github`.
3. On open / sync / reopen, Norix creates (or updates) a git workload named `pr-<number>`, queues a build from the PR head SHA, sets domain `pr-<n>.<repo>.preview.localhost`, and comments the URL on the PR when `NORIX_GITHUB_TOKEN` is set.
4. On close (or TTL expiry), the preview is marked destroyed and the workload is deleted.

## Control-plane env

| Variable | Required | Meaning |
|---|---|---|
| `NORIX_GITHUB_WEBHOOK_SECRET` | yes | HMAC secret for `X-Hub-Signature-256` |
| `NORIX_PREVIEW_ORG_ID` | yes | Organization UUID that owns preview workloads |
| `NORIX_PREVIEW_PROJECT_ID` | yes | Project UUID where `pr-*` workloads live |
| `NORIX_GITHUB_TOKEN` | for comments | PAT / app token with `issues:write` |
| `NORIX_GITHUB_API` | no | Default `https://api.github.com` |
| `NORIX_PREVIEW_TTL_SECONDS` | no | Default `86400` (1 day), max 7 days |
| `NORIX_PREVIEW_DOCKERFILE` | no | Default `Dockerfile` |
| `NORIX_PREVIEW_CONTAINER_PORT` | no | Default `80` |

Wildcard DNS for `*.preview.<your-domain>` and Caddy edge routing are needed for public HTTPS hosts; local labs use `*.preview.localhost` via the edge port (`NORIX_EDGE_HTTPS_PORT`).

## GitHub webhook

Payload URL: `https://YOUR_CP/api/webhooks/github`  
Content type: `application/json`  
Secret: same as `NORIX_GITHUB_WEBHOOK_SECRET`  
Events: **Pull requests**

UI: **Previews** lists rows from `pr_previews`. Ops GC marks expired rows destroyed about every 30s.
