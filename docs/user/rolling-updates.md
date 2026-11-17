# Rolling / zero-downtime updates

By default Norix uses a **rolling** update strategy: when a replica’s generation or image changes, the agent starts a **surge** container on a temporary host port, waits for readiness (HTTP `GET /` with TCP fallback), switches the published port, then stops the old container. The control plane refreshes domain/edge upstreams when `host_port` changes.

## Knobs (Edit tab)

| Field | Default | Meaning |
|---|---|---|
| `update_strategy` | `rolling` | `rolling` = start-before-stop; `recreate` = stop then start (brief downtime). |
| `max_surge` | `1` | Allows allocating a surge publish port. `0` with `max_unavailable=0` is coerced to surge `1` for rolling. |
| `max_unavailable` | `0` | Reserved for multi-replica wave control; per-replica rolls still prefer start-before-stop when surge &gt; 0. |

## Ops notes

- Prefer images that answer quickly on the container port (or `/`) so readiness passes before cutover.
- Full configurable probes (HTTP/TCP/exec, liveness vs readiness) are tracked in [#17](https://github.com/xiron902224/norix/issues/17).
- Combine with [release rollback](./releases-rollback.md) if a roll fails after cutover.
