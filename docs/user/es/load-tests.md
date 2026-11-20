# Pruebas de carga (k6)

Documentación canónica en inglés: [`../load-tests.md`](../load-tests.md).

## Resumen

Norix incluye scripts [k6](https://grafana.com/docs/k6/latest/) en `test/load/`:

| Script | Qué hace |
|---|---|
| `k6-smoke.js` | Solo `GET /healthz` (apto para CI con mock) |
| `k6-api.js` | Login/bootstrap, crear proyecto/workload, lecturas API y fan-out simulado de agents (`NORIX_LOAD_FULL=1`) |

```bash
# Smoke local / CI
python3 test/load/mock-healthz.py &
k6 run -e NORIX_API=http://127.0.0.1:8080 test/load/k6-smoke.js

# Lab completo (control plane arriba)
export NORIX_LOAD_FULL=1 NORIX_API=http://127.0.0.1:8080
k6 run test/load/k6-api.js
```

El fan-out de heartbeats reales (`AgentService/Heartbeat`) necesita token de nodo; el script simula presión con `/healthz` + `ListProjects` concurrentes. Detalle y resultados de ejemplo: doc EN.
