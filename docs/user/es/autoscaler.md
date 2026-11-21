# Autoscaler (GA detrás de flag) (#49)

Escalado horizontal de workloads contenedor. El **líder** del control plane ejecuta un bucle cada 10 s si `NORIX_AUTOSCALER=1`.

## Activar

```bash
export NORIX_AUTOSCALER=1
export NORIX_AUTOSCALER_MIN=1
export NORIX_AUTOSCALER_MAX=8
export NORIX_AUTOSCALER_CPU_TARGET=70
export NORIX_AUTOSCALER_MEM_TARGET=0   # % memoria del nodo; 0 = off
export NORIX_AUTOSCALER_RPS_TARGET=0
export NORIX_AUTOSCALER_COOLDOWN_MS=30000
# NORIX_AUTOSCALER_ALL=0 exige opt-in por workload (NORIX_AUTOSCALE=1)
```

## Política por workload (API / UI)

Se guarda en **variables de entorno** del workload (`UpdateWorkload` o panel Edit → Autoscaler):

| Env | Significado |
|---|---|
| `NORIX_AUTOSCALE` | `1` / `0` activar / excluir |
| `NORIX_AUTOSCALE_MIN` / `_MAX` | rango de réplicas |
| `NORIX_AUTOSCALE_CPU_TARGET` | CPU % del nodo |
| `NORIX_AUTOSCALE_MEM_TARGET` | memoria usada % del nodo |
| `NORIX_AUTOSCALE_RPS_TARGET` | métrica RPS del workload |
| `NORIX_AUTOSCALE_COOLDOWN_MS` | anti-flapping entre acciones |

UI: **Projects → app → Edit → Autoscaler**.

## Señales y flapping

CPU / memoria del nodo asignado + métricas RPS. El cooldown evita subir y bajar en el mismo ciclo. Tests: `go test ./internal/autoscaler/ -count=1`.

Guía EN: [autoscaler.md](../autoscaler.md). Límites duros: [scale-limits.md](../scale-limits.md).
