# Liveness / readiness probes

Each workload can define HTTP, TCP, or exec probes (Kubernetes-style). Defaults: readiness HTTP `GET /` on the published host port; liveness off.

## Behaviour

| Probe | On failure |
|---|---|
| **Readiness** | Replica status becomes `unready`; domain/edge routes are removed until ready again. Container stays up. |
| **Liveness** | Agent restarts the container (remove + run). |

Rolling updates (#15) reuse the readiness HTTP path (or TCP if path empty).

## Configure (UI)

Application → **Edit** → **Probes (JSON)** → Save. Example:

```json
{
  "readiness": { "type": "http", "path": "/healthz", "period_seconds": 10, "timeout_seconds": 3, "failure_threshold": 3 },
  "liveness": { "type": "tcp", "port": 0, "period_seconds": 15 }
}
```

`port: 0` means the workload host port. Exec:

```json
{ "readiness": { "type": "exec", "command": ["wget", "-qO-", "http://127.0.0.1/healthz"] }, "liveness": { "type": "none" } }
```

## Status

Summary shows **Ready** yes/no and the last `probe_status` JSON from the agent (`readiness`/`liveness` = `pass`|`fail`|`none`).
