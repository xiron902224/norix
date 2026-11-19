# Compose stacks

A **compose stack** is a `docker-compose.yml` from Git that Norix runs as one unit. The agent clones the repository and runs `docker compose up` with the file exactly as written. Volumes, `command`, `entrypoint`, healthchecks, `depends_on` conditions, `restart: "no"` jobs, secrets files and relative bind mounts behave as they do on your laptop.

## Deploy a stack

1. Open a project → **Compose stack**.
2. Fill **Stack name**, **Git URL**, **Ref** and **Compose file** (path inside the repo, for example `deploy/docker-compose.yml`).
3. Choose:
   - **Deploy stack**: queue it now. Use this for private repositories that only the agent can clone (SSH key on the node).
   - **Analyze compose**: the control plane clones the repo and lists the services first. This needs a repository the control plane can reach.
4. The stack card shows the status and every service. A node with Docker claims the stack on its next heartbeat.

What the agent runs, in `<agent workdir>/stacks/<stack id>/`:

```bash
git clone --depth 1 --branch <ref> <url> src     # later deploys: fetch + reset --hard
docker compose -p norix-<id> -f src/<compose file> -f norix.override.json \
  up -d --build --remove-orphans
```

`norix.override.json` only adds labels (`norix.stack_id`, `norix.project_id`). The checkout is kept between deploys so relative bind mounts keep their data.

## Status

| Stack status | Meaning |
|---|---|
| `pending` | Waiting for a node, or for the next heartbeat after an action. |
| `deploying` | `docker compose up` is running, or a service is still `created` / health `starting`. |
| `running` | Every service is running and not unhealthy. A one-shot job that exited with code 0 counts as **Completed**. |
| `degraded` | A service is restarting, unhealthy, or exited with a non-zero code. |
| `stopped` | The stack was stopped, or no container exists. |
| `failed` | The last apply failed (clone, `compose config`, build or `up`). The card shows the error. Fix the repo and click **Redeploy**. |
| `removing` | `docker compose down` is running. |

The build and `up` run in the background, so the agent keeps sending heartbeats during long builds. The default timeout is one hour. Change it with `NORIX_COMPOSE_UP_TIMEOUT_SECS` on the agent (60–21600).

## Actions

| Button | Agent runs |
|---|---|
| **Start** / **Redeploy** | `git fetch` + `docker compose up -d --build --remove-orphans` |
| **Stop** | `docker compose stop` |
| **Delete** | `docker compose down --remove-orphans`, then removes the checkout. **Named volumes are kept.** |

Each action bumps the stack generation (`gen observed/desired` on the card).

## Logs

**Logs** on the card shows the last lines per service, plus `deploy output` (clone, build and `up`). Lines follow the [log retention](./ops.md#log-retention) policy: each service counts like one workload.

## Placement

All stacks of a project run on the **same node**, so later they can share Docker networks across stacks. A stack stays on its node. Norix does not move it, because its volumes live there.

## Private repositories

The agent clones as the user that runs `norix-agent`, with `GIT_TERMINAL_PROMPT=0`. For private GitHub/GitLab repos, add a read-only deploy key to that user's `~/.ssh` and use an SSH URL (`git@github.com:org/repo.git`). Do not put tokens in the URL. They end up in `.git/config` on the node. Norix redacts `user:token@` from logged git output.

## Not covered yet

| Need | Issue |
|---|---|
| Shared network across stacks and `external: true` networks with aliases | [#144](https://github.com/xiron902224/norix/issues/144) |
| Stack variables for `${VAR}` interpolation and generated secrets | [#145](https://github.com/xiron902224/norix/issues/145) |
| Services internal by default; domains with HTTPS/WSS per service | [#146](https://github.com/xiron902224/norix/issues/146) |
| Purge volumes on delete | [#147](https://github.com/xiron902224/norix/issues/147) |

Until #145 lands, put defaults in the compose file (`${VAR:-default}`) or commit a non-secret `.env` next to it.

## Legacy: split into workloads

After **Analyze compose**, **Split into workloads (legacy)** still turns each service into a separate Norix workload (old behavior). That mode ignores volumes, commands, healthchecks and `restart`. Use it only for simple image-only stacks that need per-service scaling or rolling updates.
