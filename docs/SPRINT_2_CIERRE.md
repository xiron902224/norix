# Sprint 2 — Dashboard operativo embebido

## Objetivo

Servir API + UI desde un solo binario; el owner gestiona proyectos y workloads (metadatos) desde el navegador.

## Estado

**Implementado el 2026-11-09.** Siguiente: S3 (agente Rust).

## Entregables

- Dashboard autenticado con listado/creación/borrado de proyectos.
- Detalle de proyecto con CRUD de workloads (runtime container por defecto).
- Clientes Connect compartidos con interceptor Bearer.
- Renovación periódica de sesión desde el shell del dashboard.
- `go:embed` de `internal/webui/dist` servido en `/` con caché por tipo de asset.
- Dockerfile multi-stage: build Node → copia dist → build Go.
- Compose construye desde la raíz del repo.
- `make embed-web` / `make build` para desarrollo local.
- Playwright smoke (`web/e2e/smoke.spec.ts`); UI full requiere `NORIX_E2E=1` y control plane arriba.
- CI: job `build-web` + artifact incrustado en el build Go; RLS ampliado.

## DoD

| Criterio | Estado |
|---|---|
| Dashboard carga desde el binario (sin `npm run dev` en prod) | Cumple |
| Login + create project en UI | Cumple |
| `/healthz` y `/` en el mismo puerto 8080 | Cumple |
| Playwright smoke definido | Cumple (healthz siempre; UI con flag) |

## Notas

- Token de sesión sigue en `localStorage` (cookie HttpOnly diferida).
- Deploy sigue deshabilitado hasta agente enrolado.
- Placeholder `internal/webui/dist/index.html` se sustituye al hacer `make embed-web` o en el Dockerfile.
