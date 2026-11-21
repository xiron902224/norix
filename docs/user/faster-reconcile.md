# Faster reconcile (#44)

Before this change, desired state only moved on the agent heartbeat (default **10s**). After a deploy or stop, convergence waited up to one full interval.

## What changed

The agent opens `WatchDesired` (long-poll) in parallel with heartbeats:

1. Control plane returns immediately when the desired fingerprint for that node changes, or after a timeout (~25s).
2. Mutating RPCs (create/update/delete workload, compose actions) wake waiters via an in-process hub.
3. On disconnect the agent retries with backoff; a fingerprint keeps replies idempotent so reconnects do not storm applies.

## Latency (order of magnitude)

| Path | Typical wait after a desired-state change |
|---|---|
| Heartbeat only (before) | 0–10s (`NORIX_HEARTBEAT_SECS`) |
| WatchDesired (after) | tens of ms to ~1s on a healthy LAN; bounded by watch timeout if idle |

Measure locally: deploy while watching agent logs for `watch desired` / reconcile messages; compare wall time from API return to container running.

## Config

| Variable | Where | Default | Meaning |
|---|---|---|---|
| `NORIX_HEARTBEAT_SECS` | agent | `10` | Still used for liveness / capability reports |
| `NORIX_WATCH_DESIRED` | agent | on | Set `0` / `false` to disable long-poll (heartbeat-only) |
| `NORIX_WATCH_TIMEOUT_SECS` | agent | `25` (max `30`) | Client long-poll wait; CP clamps the same range |
| Watch timeout | control plane | request / 25s | Server-side long-poll ceiling |

Heartbeats remain required for node liveness; WatchDesired only accelerates desired-state delivery.
