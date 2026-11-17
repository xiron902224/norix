# Docker socket exposure

## Why it matters

`/var/run/docker.sock` is the Docker Engine API. A process that can talk to it can create privileged containers and mount the host filesystem — on a normal (rootful) daemon that is effectively **host root**.

Norix needs *some* component to start workload containers. The question is **which** container holds the socket.

## Default (recommended): agent holds the socket

In Docker install mode (`deploy/norix/docker-compose.prod.yml`):

| Service | `docker.sock` | Role |
|---|---|---|
| **control-plane** | **No** | API, UI, scheduling, desired state |
| **agent** | **Yes** | Builds + reconciles containers on this host |
| postgres | No | Data |

Trade-off: the agent (and anything that compromises it) can still reach the Docker API. The **panel/API** blast radius is smaller — an RCE in the control-plane image does not automatically get the socket.

The installer writes `NORIX_JOIN_TOKEN` / `NORIX_BOOTSTRAP_JOIN_TOKEN` into `.env`. After you create the first admin, the control plane seeds that join token so the Compose agent can enroll (retries with `restart: unless-stopped`).

## Opt-in: embedded runtime (control plane holds the socket)

Legacy one-box path: mount the socket into the control plane and set `NORIX_EMBEDDED_RUNTIME=1`.

```bash
docker compose --env-file .env \
  -f deploy/norix/docker-compose.prod.yml \
  -f deploy/norix/docker-compose.embedded-runtime.yml up -d
```

Use only on a **trusted** host. Prefer the agent default for anything internet-facing.

## Hardening options (optional)

### Socket proxy

Put [Tecnativa docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy/) (or similar) in front of the real socket and point the agent at `DOCKER_HOST=tcp://docker-proxy:2375` with an allow-list.

Caveats:

- A Norix agent that builds and runs containers needs broad API access (`CONTAINERS`, `IMAGES`, `NETWORKS`, `POST`, often `BUILD`). A proxy does **not** make that equivalent to “read-only”.
- Even “GET-only” configs can expose sensitive container data; treat proxy as **reduction**, not elimination, of risk.

### Rootless Docker

Run the Engine in [rootless mode](https://docs.docker.com/engine/security/rootless/) so daemon compromise is closer to the unprivileged user than to host root. Point the agent at the rootless socket (`$XDG_RUNTIME_DIR/docker.sock`). Some features (privileged ports, certain devices) need extra setup.

## Installer warning

`get-norix.sh` (docker mode) and `install-agent.sh` warn that mounting the socket is powerful and link here. Bare control plane never needs the socket; workers do.

## Related

- Threat model: [`docs/threat-model-s19.md`](../threat-model-s19.md)
- Nodes & agents: [`nodes-agents.md`](./nodes-agents.md)
- Issue [#10](https://github.com/xiron902224/norix/issues/10)
