# Autoscaler (GA behind flag) (#49)

Horizontal pod-style scaling for container workloads. The control-plane **leader** runs a 10s loop when `NORIX_AUTOSCALER=1`.

## Enable

```bash
# control plane
export NORIX_AUTOSCALER=1
# optional globals
export NORIX_AUTOSCALER_MIN=1
export NORIX_AUTOSCALER_MAX=8          # also clamped by NORIX_MAX_AUTOSCALER_REPLICAS (#45)
export NORIX_AUTOSCALER_CPU_TARGET=70
export NORIX_AUTOSCALER_MEM_TARGET=0   # percent node memory used; 0 = off
export NORIX_AUTOSCALER_RPS_TARGET=0
export NORIX_AUTOSCALER_COOLDOWN_MS=30000
# set NORIX_AUTOSCALER_ALL=0 to require per-workload NORIX_AUTOSCALE=1
```

## Per-workload policy (API / UI)

Policies are **workload env vars** (set via `UpdateWorkload` or the Edit → Autoscaler panel):

| Env | Meaning |
|---|---|
| `NORIX_AUTOSCALE` | `1` / `0` opt-in / opt-out |
| `NORIX_AUTOSCALE_MIN` / `_MAX` | replica bounds |
| `NORIX_AUTOSCALE_CPU_TARGET` | scale when node CPU % above target |
| `NORIX_AUTOSCALE_MEM_TARGET` | scale when node memory used % above target |
| `NORIX_AUTOSCALE_RPS_TARGET` | scale when `workload_rps` / `workload_http_rps` metric exceeds target |
| `NORIX_AUTOSCALE_COOLDOWN_MS` | minimum time between scale actions (anti-flap) |

Dashboard: **Projects → app → Edit → Autoscaler**.

## Signals

- CPU: assigned node `cpu_usage_percent` / metric `node_cpu_usage_percent`
- Memory: `(memory_total_mb - memory_free_mb) / memory_total_mb` (and optional `node_memory_usage_percent` sample)
- RPS: latest `metric_samples` named `workload_rps` or `workload_http_rps` labeled `workload_id`

## Flapping

Cooldown blocks scale-down immediately after scale-up (and vice versa). Unit tests: `go test ./internal/autoscaler/ -count=1` (`TestFlappingBlockedByCooldown`).

## Limits

Autoscaler max cannot exceed `NORIX_MAX_AUTOSCALER_REPLICAS` / `NORIX_MAX_REPLICAS` — see [scale-limits.md](./scale-limits.md).
