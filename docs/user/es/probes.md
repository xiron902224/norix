# Probes de liveness / readiness

Cada workload puede definir probes HTTP, TCP o exec (estilo Kubernetes). Por defecto: readiness HTTP `GET /` en el puerto host; liveness desactivado.

## Comportamiento

| Probe | Si falla |
|---|---|
| **Readiness** | Status `unready`; se quitan rutas de dominio/edge hasta recuperar. El contenedor sigue arriba. |
| **Liveness** | El agent reinicia el contenedor (remove + run). |

Los rolling updates (#15) reutilizan el path HTTP de readiness (o TCP si el path está vacío).

## Configurar (UI)

Application → **Edit** → **Probes (JSON)** → Save. Ejemplo:

```json
{
  "readiness": { "type": "http", "path": "/healthz", "period_seconds": 10, "timeout_seconds": 3, "failure_threshold": 3 },
  "liveness": { "type": "tcp", "port": 0, "period_seconds": 15 }
}
```

`port: 0` = puerto host del workload.

## Estado

Summary muestra **Ready** sí/no y el último JSON `probe_status` del agent.
