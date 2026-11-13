# Sprint 7 — Scheduler multi-nodo

## Objetivo (plan S0–S22)

Scheduler con capacidades, spread/binpack, réplicas multi-nodo, cordon/drain, detección de nodo caído y reprogramación, roles `worker` / `edge`.

## Estado

**Cerrado 2026-11-13** (alineado al plan canónico).  

> Nota: un cierre anterior en este archivo documentaba **ops Alpha** (backups/métricas). Ese trabajo sigue vigente en código y en `docs/runbooks/S7_OPERATIONS.md`, pero en el roadmap canónico corresponde a **S9/S10**, no a S7.

## Entregables

- Paquete `internal/scheduler`: filtros (online, cordon, drain, runtime, KVM, memoria, affinity/anti-affinity) + score spread/binpack/zona.
- Roles de nodo `worker` | `edge` (vacío → worker); `control` no agenda workloads.
- `PickScheduledNode` / `DeployWorkloadReplicas` / `RescheduleOrphanReplicas`.
- Cordon / drain vía API + UI Nodes.
- Nodos stale → `offline` (30s) + reschedule de huérfanos en tick 15s (DoD &lt; ~60s).
- **Fix:** réplicas `desired_action=none` + `status=running` en nodo offline ahora sí se reprograman.
- Tests unitarios en `internal/scheduler` (KVM, spread/binpack, drain, roles, orphan eligibility).

## DoD

| Criterio | Estado |
|---|---|
| Spread / binpack | Cumple (unit) |
| Cordon / drain | Cumple |
| Micro-VM (`RequireKVM`) no va a nodo sin KVM | Cumple (unit) |
| Nodo offline → réplicas reprogramables | Cumple (SQL + `ShouldReschedule`) |
| Roles worker/edge | Cumple |
| Tests verdes | `go test ./internal/scheduler/…` |

## Cómo verificar

```bash
cd control-plane && go test ./internal/scheduler/ ./internal/db/ ./internal/api/ -count=1
```

Demo multi-nodo (manual): 2 agentes online → workload `replica_count=2` strategy spread → cordon/drain uno → matar heartbeat del otro → en &lt; 60s réplicas en nodo sano.
