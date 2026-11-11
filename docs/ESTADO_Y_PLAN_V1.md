# Norix — Estado del proyecto, mejoras y plan de sprints hasta v1

**Fecha:** 2026-11-09 (actualizado al cerrar S1c e implementar S2)  
**Alcance:** análisis verificable del repositorio + mejoras de lo ya implementado + replanificación de sprints hasta GA 1.0  
**Documentos relacionados:** `Arquitectura final de la plataforma PaaS.md`, `Plan de sprints de cero a producción.md`, `SPRINT_1_CIERRE.md`, `PLAN_NORIX_APRENDIZAJE.md`, `ARQUITECTURA_Y_BUENAS_PRACTICAS.md`

---

## 1. Veredicto

Norix cerró **S1** y entregó **S2**: control plane Go con auth/RLS/RBAC, projects/workloads, SPA React de proyectos embebible en el binario, CI ampliado. **No hay runtime**: el agente Rust sigue vacío; `Deploy` falla explícitamente hasta S3/S4.

Sin el agente, el producto sigue siendo un panel de metadatos, no una PaaS de ejecución.

---

## 2. Mapa de lo implementado vs vacío

| Componente | Estado | Evidencia |
|---|---|---|
| `proto/` (auth, workload, agent, metrics) | Contratos definidos | Servicios Connect generados en Go; TS solo mensajes (`*_pb.ts`) |
| `control-plane/` | **Operativo (MVP)** | Auth, projects, workloads, middleware Bearer, rate limit, migraciones, Docker |
| `web/` | **Shell auth** | Bootstrap + login + logout; sin router, proyectos ni dashboard real |
| `deploy/norix/` | **Local OK** | Compose Postgres + control-plane |
| `scripts/install.sh` | **MVP local** | Genera `.env` y levanta Compose; sin firma, upgrade ni uninstall |
| `agent/` | **Vacío** | Solo README + `.env.example` |
| `cli/`, `sdk/`, `builder/`, `test/` | **Vacío** | Carpetas placeholder |
| `images/kernel`, `images/rootfs` | **Stub** | Preparación micro-VM sin pipeline |
| ADRs / threat model | **Ausentes** | Pedidos en S0, no hay `docs/adr/` |
| Spikes KVM / Wasm / Caddy | **No cerrados** | Hay `deploy/caddy-pebble/` pero sin informe de decisión |

### Control plane — qué sí hay

- Servidor HTTP + Connect bajo `/api/`, `/healthz`, graceful shutdown.
- PostgreSQL 16, roles `norix` / `norix_app`, migraciones embebidas (auth, projects/workloads, audit).
- Auth: bootstrap owner (advisory lock), login Argon2id, sesión opaca hasheada SHA-256, logout, renew.
- Middleware Bearer en procedimientos no públicos.
- Rate limiting in-memory en login/bootstrap.
- ProjectService: create / list / delete + auditoría en create/delete.
- WorkloadService: CRUD + `Deploy` que **solo marca** `status=deploying` (sin runtime).
- CI: `buf lint`/`breaking`, migraciones+RLS parcial, `go test`, build estático.
- Frontend React 19 + Vite: bootstrap, login, logout localStorage.

### Lo que la docs marca “hecho” pero sigue incompleto o frágil

| Ítem | Problema |
|---|---|
| Auditoría | Tabla + insert en projects; workloads/auth no auditan; sin `actor_user_id` rellenado |
| RBAC | Enum `Role` en proto; enforcement real casi inexistente (solo “hay sesión”) |
| Deploy | Stub de estado; no hay deployment_id real ni agente |
| AgentService / MetricsService | Proto existe; **no hay handlers registrados** en `main.go` |
| sqlc / SQLite dual | Documentado en plan original; el código usa `pgx` a mano y solo Postgres |
| Next.js / SSR | Plan semanal antiguo; arquitectura final ya eligió SPA embebida |
| Embebido `go:embed` | No implementado; web corre aparte en Vite |
| CI web / Playwright | No hay job de frontend ni e2e |
| RLS en CI | Verifica 4 tablas; no incluye `workloads` ni `audit_events` |
| Sesión en frontend | Token en `localStorage` (XSS-sensitive); plan pedía cookie HttpOnly |
| `buf.gen` TS | Solo protobuf-es; clientes Connect se crean a mano con `createClient(Service)` |
| Docs de progreso | `PLAN_NORIX_APRENDIZAJE.md` lista frontend/logout como pendientes aunque ya existen |

---

## 3. Mejoras de lo ya implementado (prioridad y por qué)

Estas mejoras **no son features nuevas de PaaS**; endurecen la base antes de construir el agente. Hacerlas ahora evita reescribir auth, multi-tenant y contratos cuando el stream del agente exista.

### P0 — Correctitud y seguridad (hacer antes de S3 agente)

1. **Cerrar S1 con checklist de aceptación real**  
   Instalación limpia → bootstrap UI → login tras reinicio → Bearer válido/inválido → aislamiento RLS entre dos orgs.  
   *Por qué:* sin demo reproducible, el resto del roadmap es deuda escondida.

2. **Completar auditoría transaccional**  
   Actor (user id/email), acciones de workloads, login fallido agregado (sin filtrar contraseñas), y lectura API/admin.  
   *Por qué:* sin traza de actor no hay forensics ni beta privada creíble.

3. **Autorización por rol, no solo autenticación**  
   Owner/admin/member en Project/Workload; denegar delete a member.  
   *Por qué:* el modelo de datos ya tiene `role`; ignorarlo invita a bugs de privilege escalation cuando haya invitaciones.

4. **Tenant context unificado**  
   Helper único `WithTenant(tx, orgID)` + tests que fallen si se olvida `set_config`.  
   *Por qué:* RLS solo protege si *todas* las rutas lo setean; hoy está duplicado a mano.

5. **Sesiones: limpieza y renew desde el cliente**  
   Job/cron o query al arranque que borre expiradas; frontend renueva antes de caducar.  
   *Por qué:* tabla de sesiones crece; UX rompe a las 12 h sin aviso.

6. **Rate limit por IP + persistencia o ventana compartida**  
   Hoy es memoria de proceso; reinicios y multi-réplica lo anulan.  
   *Por qué:* bootstrap/login son superficie pública.

7. **No filtrar errores internos de DB al cliente**  
   Varios handlers pasan `err.Error()` como `CodeInternal`/`InvalidArgument`.  
   *Por qué:* fuga de esquema/SQL y ruido de seguridad.

### P1 — Producto usable del panel (S2)

8. **Dashboard real: proyectos y workloads**  
   List/create/delete wired al API existente; estados visibles.  
   *Por qué:* la API ya existe y no se ejercita desde UI.

9. **Router + Query (TanStack) y layout de app**  
   Separar auth shell del área autenticada.  
   *Por qué:* `App.tsx` monolítico no escala a logs/nodos/deploys.

10. **`go:embed` del `web/dist` + headers de caché**  
    Binario único en Compose/producción.  
    *Por qué:* presupuesto de arquitectura (“0 procesos Node en prod”) y simplifica install.

11. **Cookies HttpOnly Secure SameSite (o BFF cookie)**  
    Sustituir o complementar `localStorage`.  
    *Por qué:* reduce robo de sesión por XSS en el dashboard.

12. **CI: build web + lint + smoke Playwright (bootstrap/login)**  
    *Por qué:* hoy CI no protege el frontend.

13. **CI RLS completo**  
    Incluir `workloads`, `audit_events`; test negativo cross-tenant.  
    *Por qué:* la migración 2/3 ya existe pero CI no la valida del todo.

### P2 — Higiene de ingeniería (paralelo)

14. **Una sola fuente de verdad documental**  
    Marcar obsoletos: plan Next.js, dual SQLite, sqlc obligatorio. Actualizar `PLAN_NORIX_APRENDIZAJE` y `SPRINT_1_CIERRE`.  
    *Por qué:* planes contradictorios generan thrash (p. ej. gRPC puro vs Connect).

15. **ADRs mínimos (001–010 o al menos los decididos)**  
    Go/Rust split, Connect, Postgres-only v1, React SPA embed, containerd, Caddy, (CH vs Firecracker pendiente de spike).  
    *Por qué:* sin decisión escrita se reabre el debate cada sprint.

16. **Alinear `Deploy` con semántica real**  
    Hasta tener agente: devolver `FailedPrecondition` (“no runtime enrolled”) en lugar de fingir `deploying`.  
    *Por qué:* estados mentirosos contaminan UI y tests futuros.

17. **Quitar o stubear protos no cableados**  
    Agent/Metrics: o registrar handlers no-op documentados, o marcarlos `reserved`/WIP en docs.  
    *Por qué:* contratos “muertos” confunden generación multi-lenguaje.

18. **sqlc como migración opcional, no bloqueante**  
    El plan original lo exige; el código ya funciona con `pgx`. Adoptar sqlc *después* de estabilizar esquema de nodos/desired state.  
    *Por qué:* reescribir capa DB ahora atrasa el agente sin ganar runtime.

---

## 4. Desalineaciones documentales a resolver

| Tema | Docs antiguas | Arquitectura / código actual | Decisión para v1 |
|---|---|---|---|
| Frontend | Next.js SSR | React+Vite SPA | **SPA embebida** (ya decidido en arquitectura final) |
| DB | SQLite + Postgres + sqlc | Postgres + SQL manual | **Postgres-only hasta multi-nodo**; sqlc opcional post-S4 |
| API | gRPC / GraphQL mezclados | ConnectRPC | **ConnectRPC** |
| Contenedores | Docker socket / bollard | containerd (arquitectura) | **containerd** (mantener) |
| Equipo | 4 personas, ~11 meses | Ritmo aprendizaje / 1–2 devs | Plan reescalado abajo |
| v1 GA | Incluye Wasm + HA + SSO | Aún sin agente | **v1.0 = Alpha endurecida + multi-tenant básico + un nodo productivo**; Wasm/HA/SSO = v1.1–v1.2 |

### Definición de versión 1.0 (recorte consciente)

Para lanzar algo defendible sin diluir 22 sprints de visión máxima:

**Norix v1.0 (GA mínima)** = instalación en un VPS, auth/RLS, dashboard, **un agente Rust enrolado**, deploy de contenedores desde imagen (y idealmente git→build), dominio + HTTPS, secretos cifrados, logs básicos, backup Postgres, install/upgrade documentado.

Queda **fuera de v1.0** (roadmap post-GA / v1.x):

- Micro-VMs / sandboxes / PR previews (diferenciador, pero no bloquea “PaaS usable”)
- Wasm de primera clase
- WireGuard multi-nodo / HA Patroni
- SSO/SAML, cuotas enterprise, SDKs completos

Esos ítems se mantienen en el plan como **Fase post-v1** para no perder la visión, con sprints numerados después de GA.

---

## 5. Estado de sprints (antes → ahora)

| Sprint original | Intención | Estado real |
|---|---|---|
| S0 fundaciones | Monorepo, CI, spikes, ADRs | **Parcial**: monorepo+CI; spikes/ADRs pendientes |
| S1 control plane | Auth, projects, audit, RBAC | **Cerrado (S1c)** |
| S2 dashboard | SPA + embed + Playwright | **Entregado**: projects/workloads UI, `go:embed`, smoke e2e |
| S3 agente | Enrolamiento + heartbeat | **Entregado**: join token, Rust agent, nodes UI |
| S4 contenedores | Desired→observed + Deploy | **Entregado**: reconciler Docker/nerdctl, logs UI |
| S5 builds/git | Git → build → registry | **Entregado**: DeployFromGit, zot, build logs, webhook |
| S6 edge/TLS | Dominio + HTTPS (Alpha 0.1) | **Entregado**: Caddy/Pebble, Domains UI, rollback |
| S7 ops | Backup/métricas/alertas | **Entregado**: Ops UI, backups, node metrics, alerts |
| S8–S10 (replan) | Hardening → GA 1.0 | **Pendiente** — siguiente: S8 hardening |

---

## 6. Replan de sprints hasta v1.0

**Supuestos**

- Ritmo: **sprints de 2 semanas**, 1–2 personas (full-stack + systems).
- No empezar el siguiente sprint si el DoD del actual falla.
- Cada sprint entrega artefacto instalable (`install.sh` o Compose) + demo.
- Este plan **sustituye** el calendario operativo de `Plan de sprints de cero a producción.md` hasta GA; la arquitectura final sigue siendo la norte técnica.

**Calendario objetivo:** ~**20 semanas** (10 sprints) hasta **v1.0 GA mínima**, luego fases post-v1.

```text
Ahora ── S1c cierre ── S2 dashboard ── S3 agente ── S4 contenedores
      ── S5 builds/git ── S6 edge/SSL (Alpha 0.1)
      ── S7 ops/backup/observabilidad ── S8 hardening
      ── S9 beta privada ── S10 GA 1.0
```

### S1c · Cierre de fundaciones (1 sprint corto o 1 semana)

**Objetivo:** declarar S1 cerrado con evidencia, no con checklist teórico.

**Entregables**

- Script/demo: install limpio → bootstrap UI → login → create project API → logout.
- Tests integración RLS cross-tenant (positivo/negativo).
- Auditoría con actor; RBAC mínimo en mutate.
- `Deploy` responde “no agent” hasta S3/S4.
- Actualizar `SPRINT_1_CIERRE.md` y este doc; ADR-001..005 borrador.
- CI: verificar `workloads` + `audit_events` + job `web` build.

**DoD:** los 10 criterios de `SPRINT_1_CIERRE.md` en verde, o explícitamente deferidos con dueño (solo embed → S2).

### S2 · Dashboard operativo embebido

**Objetivo:** un solo binario sirve API + UI; el owner gestiona proyectos/workloads (metadatos).

**Entregables**

- TanStack Router/Query (o equivalente ligero ya alineado al repo).
- Páginas: login/bootstrap, projects, project detail/workloads.
- Cliente Connect para Project/Workload.
- `go:embed` + compresión/caché.
- Playwright: bootstrap + login + create project.
- Sesión: renovación; preferencia cookie HttpOnly si el diseño de auth lo permite sin romper Compose local.

**DoD:** `curl /healthz` + abrir `/` en el mismo puerto 8080 sin `npm run dev`; e2e verde en CI.

### S3 · Agente Rust, enrolamiento y stream

**Objetivo:** un nodo aparece vivo en el control plane.

**Entregables**

- Workspace Cargo, toolchain pinned, CI Rust.
- Join token de un solo uso → certificado mTLS (o MVP: token + TLS server-side primero, mTLS completo si cabe).
- Stream saliente bidireccional (`AgentService`) con heartbeat y reconexión backoff.
- Capacidad reportada: CPU/RAM, docker/containerd socket, KVM level 0–2 (detección; boot real puede deferirse).
- UI: lista de nodos + last_seen.
- `install-agent.sh`.

**DoD:** matar red 30 s → agente reconecta; nodo visible; agente < 50 MB en reposo (medir).

### S4 · Reconciliación + runtime contenedores

**Objetivo:** desired state → contenedor real.

**Entregables**

- Modelo desired/observed versionado en proto + DB.
- Runtime containerd (crear/start/stop/logs).
- Reconciler idempotente en el agente.
- `Deploy` real desde dashboard (imagen pública).
- Stream de logs mínimo (MetricsService o canal del agent stream).

**DoD:** deploy nginx (o similar), HTTP alcanzable en puerto mapeado; matar agente a mitad y convergencia al volver.

### S5 · Git, build y registry

**Objetivo:** git push (o “deploy from repo”) produce imagen y despliegue.

**Entregables**

- Webhook GitHub (GitLab básico si hay tiempo).
- Clone + detect Dockerfile (Buildpacks opcional).
- BuildKit + registry zot local.
- Logs de build en UI.
- Secretos de build cifrados en reposo (age o libsodium; ADR).

**DoD:** repo ejemplo Node o Go → imagen → running.

### S6 · Edge, dominio, TLS — **Alpha v0.1**

**Objetivo:** app con HTTPS en VPS limpio.

**Entregables**

- Caddy controlado por API desde el agente (un solo escritor, rollback de config).
- Dominio/subdominio → workload.
- ACME (Pebble en dev, staging LE en prueba).
- Health check + rollback simple de release.
- `install.sh` upgrade/uninstall documentado; checksums.

**DoD:** VPS limpio < 15 min (ideal < 3 cuando madure) → git/image → HTTPS. **Demo Alpha pública interna.**

### S7 · Operación: backups, logs, métricas mínimas — **ENTREGADO**

**Objetivo:** operar Alpha sin miedo. Ver `docs/SPRINT_7_CIERRE.md` y `docs/runbooks/S7_OPERATIONS.md`.

**Entregables**

- Backup/restore Postgres (UI Ops + scripts).
- Retención de logs de workload/build (worker ops).
- Métricas nodo (CPU/RAM/disco) en Nodes.
- Alertas mínimas: nodo down, deploy failed (webhook opcional).
- Runbook: restore, rotar secretos, upgrade.

**DoD:** restore documentado; alerta al parar el agente remoto (~45s).

### S8 · Hardening y superficie de ataque

**Objetivo:** beta-ready security.

**Entregables**

- Threat model STRIDE corto + ADRs restantes.
- Revisión secretos, permisos Unix, non-root, capabilities.
- Rate limits / lockout; rotación de join tokens.
- SBOM del control plane + agente; firma de release (cosign/minisign).
- Fuzz o property tests en parsers de config/proto críticos.
- Pentest ligero checklist (authz, RLS, path traversal en logs).

**DoD:** informe de hallazgos P0/P1 cerrados; releases firmados.

### S9 · Beta privada v0.5

**Objetivo:** 5–10 usuarios externos en installs reales.

**Entregables**

- Onboarding docs (quickstart, troubleshooting).
- Invites / segundo usuario (member) si aún no existe.
- Telemetría opt-in de errores de install (sin secretos).
- Freeze de features: solo bugs y UX bloqueante.
- Plantilla 1-click Postgres **opcional** (si no llega, documentar como v1.1).

**DoD:** ≥5 installs exitosos reportados; lista de issues priorizada para GA.

### S10 · GA v1.0

**Objetivo:** lanzamiento.

**Entregables**

- Versionado semver, changelog, upgrade path N-1→N.
- SLOs iniciales (availability install, deploy success rate).
- Docs oficiales: arquitectura resumida, seguridad, límites de v1.
- Sitio/README de producción; demos grabadas.
- Tag `v1.0.0` + artefactos multi-arch (x86_64, aarch64).

**DoD:** checklist GA firmado; comunicación de “qué no incluye v1” (micro-VM, Wasm, HA).

---

## 7. Post-v1 (visión conservada, no bloquea el lanzamiento)

| Fase | Sprints | Contenido |
|---|---|---|
| v1.1 Sandboxes | P1–P3 | Micro-VM Cloud Hypervisor, guest agent, API sandbox, snapshots |
| v1.2 Platform | P4–P5 | PR previews, SDKs/CLI ricos, DB 1-click completo |
| v1.3 Edge scale | P6–P7 | Multi-nodo scheduler, WireGuard, certs centralizados |
| v1.4 Wasm + enterprise | P8–P9 | Wasmtime workers, OIDC/2FA, cuotas, HA Postgres |

Los spikes KVM/Firecracker/Wasm de S0 original se ejecutan **como spikes acotados dentro de S3 (detección) y P1 (runtime)**, no como prerrequisito que bloquee Alpha.

---

## 8. Riesgos activos

| Riesgo | Impacto | Mitigación |
|---|---|---|
| Alcance “Coolify + micro-VM + Wasm + HA” en un solo GA | Nunca lanzar | Recorte v1.0 de la §4 |
| Agente Rust desde cero | Retraso S3–S4 | MVP stream+heartbeat primero; containerd después; ADR “¿agente Go?” solo si CI/fricción lo exige (como el plan original) |
| Docs contradictorias | Retrabajo | Este doc = plan operativo; archivar/marcar legacy |
| `Deploy` stub | Falsa sensación de progreso | Error explícito hasta runtime |
| Token en localStorage | Sesiones robables | Cookie HttpOnly en S2 |
| Un solo desarrollador | Velocidad ~⅓ del plan de 4 | Sprints 2 semanas; DoD estricto; no paralelizar micro-VM con builds |
| KVM no disponible en VPS baratos | Bloqueo si micro-VM es “v1” | v1.0 solo contenedores; micro-VM post-GA |

---

## 9. Métricas de avance sugeridas

Revisar al cierre de cada sprint:

1. % DoD del sprint en verde  
2. Tiempo install limpio (minutos)  
3. Tests automatizados nuevos / flaky  
4. Deuda documentada (issues P0 abiertos)  
5. RAM reposo control-plane y agente  
6. (Desde S4) tiempo deploy imagen pública  
7. (Desde S6) tiempo hasta HTTPS en VPS  

---

## 10. Acciones inmediatas (esta semana)

1. Ejecutar y pegar evidencia del flujo bootstrap/login en `docs/progress/`.  
2. Arreglar CI RLS (`workloads`, `audit_events`) + build `web`.  
3. Cambiar `Deploy` a `FailedPrecondition` sin agente.  
4. Redactar ADR-001 (stack) y ADR-002 (Postgres-only v1).  
5. Empezar S2: listado de proyectos en UI contra API existente.  
6. Marcar en README que el plan operativo vigente es **este documento**.

---

## 11. Resumen ejecutivo

- **Fortaleza actual:** auth sólida, RLS, contratos Buf, Compose, CI Go/proto, base de projects/workloads.  
- **Debilidad crítica:** no hay plano de ejecución (agente/runtime); UI no consume el dominio; docs infladas vs código.  
- **Mejora clave:** cerrar S1 de verdad, dashboard embebido, luego agente→contenedores→TLS antes de cualquier micro-VM.  
- **v1.0:** PaaS de un nodo con contenedores + HTTPS + operación básica en ~10 sprints; diferenciadores KVM/Wasm después.

*Fin del documento operativo. Actualizar la sección 5 y 6 al cerrar cada sprint.*
