# Sprint 1 — Esqueleto del control plane (CERRADO)

## Objetivo

Entregar un control plane Go mínimo con API Connect, autenticación persistente, PostgreSQL, bootstrap del administrador inicial y sesión operativa.

## Estado

**Cerrado el 2026-11-09** (S1c). El plan operativo vigente continúa en `docs/ESTADO_Y_PLAN_V1.md` → Sprint 2.

## Completado

- Servidor HTTP/ConnectRPC con `/healthz` y graceful shutdown.
- PostgreSQL 16, migraciones embebidas, roles `norix` / `norix_app`, RLS forzado.
- Auth: bootstrap owner (advisory lock), login Argon2id, sesiones opacas SHA-256, logout, renew, cleanup periódico.
- Middleware Bearer en rutas no públicas; rate limit en login/bootstrap.
- Project CRUD + auditoría con actor; Workload CRUD.
- RBAC mínimo: solo `owner`/`admin` mutan; `member` puede listar.
- `Deploy` responde `FailedPrecondition` hasta que exista un agente enrolado.
- Errores internos sin filtrar SQL al cliente.
- Frontend React: bootstrap, login, logout.
- CI: buf, migraciones, RLS (`organizations`, `users`, `sessions`, `projects`, `workloads`, `audit_events`), test de aislamiento cross-tenant, build Go.
- Dockerfile multi-stage (ampliado en S2 con UI embebida).

## Criterios de cierre (DoD)

| # | Criterio | Estado |
|---|---|---|
| 1 | Instalación limpia con Compose / install.sh | Cumple (local) |
| 2 | Owner creado desde frontend | Cumple |
| 3 | Token y owner devueltos por bootstrap | Cumple |
| 4 | Login persistente tras reinicio | Cumple |
| 5 | Logout revoca sesión | Cumple |
| 6 | Bearer válido aceptado / inválido rechazado | Cumple |
| 7 | Tenant no lee otro tenant con `norix_app` | Cumple (test CI) |
| 8 | Proyecto y workload CRUD mínimos | Cumple |
| 9 | Auditoría con actor/recurso/acción | Cumple |
| 10 | Go, frontend y Buf en CI | Cumple (web job en S2) |

## Diferido a S2+

- SPA embebida con `go:embed` (S2).
- Dashboard de proyectos en UI (S2).
- Cookies HttpOnly (mejora de sesión; token localStorage permanece en S2 inicial).
- Runtime agent / deploy real (S3–S4).

## Arquitectura entregada

```text
cmd/server → api (Connect) → auth / db → PostgreSQL + RLS
web (Vite) → Connect clients → /api
```

## Próximo sprint

S2: dashboard operativo embebido en el binario Go.
