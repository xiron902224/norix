# Norix — Estado y plan hasta v1.0 GA

**Fecha:** 2026-11-13  
**Fuente de verdad operativa:** este documento.  
**Relacionados:** `docs/adr/`, `docs/SPRINT_*_CIERRE.md`, `docs/runbooks/S7_OPERATIONS.md`, `docs/AGENT_PRODUCTION_CHECKLIST.md`, `docs/ARQUITECTURA_Y_BUENAS_PRACTICAS.md`

---

## 1. Veredicto

Norix está en **Alpha operativa**: un solo nodo (control plane + local runtime Docker y/o agente Rust) puede autenticar, desplegar imagen/git, enrutar HTTPS local (Caddy + Pebble) y operar backups/métricas/alertas básicas.

**Siguiente bloque hasta producción:** S8 hardening → S9 beta privada → S10 GA v1.0.

Hay **scaffolds post-v1** en el árbol (sandboxes, Wasm, PR previews, teams/quotas, managed DBs, HA leases). Existen APIs/UI/migraciones, pero **no son DoD de v1.0**: no bloquean GA y no sustituyen hardening.

---

## 2. Qué hay en el código (verificado)

### Núcleo v1 (listo para endurecer)

| Área | Estado | Evidencia |
|---|---|---|
| Auth / sesiones / bootstrap | Hecho | migraciones 000001+, Connect AuthService |
| Projects / workloads / RLS | Hecho | 000002–000003, Project/Workload services |
| Agente Rust | Hecho (mejorable) | enroll, heartbeat, reconcile Docker/nerdctl |
| Local runtime embebido | Hecho | `internal/localruntime` (deploy sin agente remoto) |
| Desired → observed + scheduler | Hecho | 000005, 000010–000011; self-heal si falta contenedor |
| Git → build → imagen | Hecho | 000007, DeployFromGit, zot opcional |
| Dominios + TLS local | Hecho | 000008, Caddy admin, retry de rutas `failed`, resync post-compose |
| Ops (backup, métricas, alertas) | Hecho | 000009, Ops UI, runbook S7 |
| Network hooks | Parcial | 000012 + UI Network; WireGuard apply opt-in |
| Dashboard React/Vite + embed | Hecho | `web/` + `go:embed` dist |
| CLI / SDK | Esqueleto | `cli/`, `sdk/` — no son interfaz primaria aún |

### Scaffolds post-v1 (presentes, no GA)

| Área | Migración / API | Nota |
|---|---|---|
| Observability extra | 000013, MetricsService, `/metrics` | Ampliar en ops; no bloquea GA |
| Managed databases | 000014 + UI Databases | Útil; cerrar provision + secrets en post-GA o S9 si cabe |
| Sandboxes / micro-VM | 000015 | Visión v1.1 — no bloquear GA |
| PR previews | 000016 | Visión v1.2 |
| Wasm | 000017 + agent `wasm.rs` | Visión v1.4 |
| Teams / quotas / SSO/TOTP stubs | 000018 + UI Teams | Enterprise — post-GA salvo invites mínimos en S9 |
| HA leases | 000019 | Multi-control-plane — post-GA |

### Definición de v1.0 GA (recorte consciente)

Instalable en un VPS: auth+RLS, dashboard, un runtime (agente o local), deploy imagen y git→build, dominio+HTTPS, secretos de build cifrados, logs/ops básicos, backup/restore Postgres, install/upgrade documentado, **hardening S8 cerrado**, beta S9 con installs reales.

**Fuera de v1.0:** micro-VM sandboxes, Wasm first-class, WireGuard multi-nodo, HA Patroni, SSO/SAML completo, SDKs ricos.

---

## 3. Sprints cerrados

| Sprint | Objetivo | Estado |
|---|---|---|
| S0 | Monorepo, CI, spikes | Parcial (ADRs mínimos sí; spikes KVM históricos deferidos) |
| S1 | Control plane auth/RLS | Cerrado (`SPRINT_1_CIERRE.md`) |
| S2 | Dashboard embebido | Entregado |
| S3 | Agente enroll/heartbeat | Entregado |
| S4 | Contenedores reconcile | Entregado (+ self-heal 2026-11-13) |
| S5 | Git/build/registry | Entregado |
| S6 | Edge/TLS Alpha 0.1 | Entregado (+ retry/resync edge 2026-11-13) |
| S7 | Ops backup/métricas/alertas | Entregado |
| **S8** | Hardening | **Siguiente** |
| S9 | Beta privada 0.5 | Pendiente |
| S10 | GA 1.0 | Pendiente |

---

## 4. Plan hasta producción

Cadencia: sprints ~2 semanas, 1–2 personas. No abrir el siguiente si el DoD del actual falla.

```text
Ahora (Alpha) ── S8 hardening ── S9 beta privada ── S10 GA v1.0
                      │
                      └── agent checklist (docs/AGENT_PRODUCTION_CHECKLIST.md)
```

### S8 · Hardening (beta-ready)

**Objetivo:** superficie de ataque y operación segura en un nodo.

**Entregables**

1. Threat model STRIDE corto + ADRs pendientes (edge, agent identity, secrets).
2. Agente: cerrar checklist de producción (arranque `--check`, recover, degraded mode, E2E).
3. Authz RBAC en todas las mutaciones; auditoría con actor completo.
4. Rate limit / lockout persistente o compartido; rotación de join tokens.
5. Secretos: permisos Unix, non-root, `NORIX_BUILD_SECRETS_KEY` obligatorio en prod.
6. SBOM + firma de release (cosign/minisign); checklist pentest RLS/path traversal.
7. Congelar o feature-flag scaffolds post-v1 que no estén listos (sandboxes/Wasm/HA) para no confundir beta.

**DoD:** hallazgos P0/P1 cerrados; install limpio documentado; agente no arranca en estado ambiguo.

### S9 · Beta privada v0.5

**Objetivo:** 5–10 installs externos reales.

**Entregables**

- Quickstart + troubleshooting actualizados (README + este doc).
- Invites / segundo usuario (member) si aún falta.
- Freeze de features: solo bugs y UX bloqueante.
- Managed Postgres 1-click **opcional** si estabiliza; si no, documentar como v1.1.
- Telemetría de errores de install opt-in (sin secretos).

**DoD:** ≥5 installs exitosos reportados; lista priorizada para GA.

### S10 · GA v1.0

**Objetivo:** lanzamiento.

**Entregables**

- Semver, changelog, upgrade N-1→N.
- Artefactos multi-arch; checksums en `install.sh`.
- Docs: límites de v1, seguridad, arquitectura resumida.
- Tag `v1.0.0`.

**DoD:** checklist GA firmado; comunicación explícita de lo que **no** incluye v1.

---

## 5. Deuda activa (priorizada)

| ID | Ítem | Sprint |
|---|---|---|
| D1 | Self-heal workloads/edge (hecho 2026-11-13) — vigilar regresiones | — |
| D2 | Dependencias sidecar (p. ej. Redis del sample todo) no gestionadas por Norix | S8/S9 docs |
| D3 | Token en `localStorage` (XSS) → cookie HttpOnly preferible | S8 |
| D4 | CLI incompleta | S9/S10 |
| D5 | ADRs incompletos vs visión | S8 |
| D6 | Scaffolds post-v1 sin DoD → flags o docs “experimental” | S8 |
| D7 | CI: cobertura web/e2e y RLS completa en todas las tablas nuevas | S8 |

---

## 6. Cómo levantar local (smoke)

```bash
cp .env.example .env
cp control-plane/.env.example control-plane/.env   # ajustar NORIX_BACKUP_DIR
cp web/.env.example web/.env

docker compose --env-file .env -f deploy/norix/docker-compose.yml up -d postgres pebble caddy

cd control-plane && set -a && . ./.env && set +a && go run ./cmd/server
# otra terminal
cd web && npm install && npm run dev -- --host 127.0.0.1 --port 5173
```

| Check | Esperado |
|---|---|
| `curl -s http://127.0.0.1:8080/healthz` | 200 |
| Dashboard | http://127.0.0.1:5173 |
| Deploy `nginx:alpine` | contenedor `norix-*` Up |
| Dominio `*.localhost:8443` | 200 tras Assign (cert interno) |

---

## 7. Mapa de documentación

| Documento | Rol |
|---|---|
| **Este archivo** | Plan operativo y estado |
| `docs/adr/*` | Decisiones vinculantes |
| `docs/SPRINT_*_CIERRE.md` | Evidencia histórica de DoD |
| `docs/runbooks/S7_OPERATIONS.md` | Backup/restore/alertas |
| `docs/AGENT_PRODUCTION_CHECKLIST.md` | Trabajo S8 del agente |
| `docs/ARQUITECTURA_Y_BUENAS_PRACTICAS.md` | Convención de carpetas/código |
| README.md | Quick start público |

Planes legacy (Next.js, calendarios de 23 sprints, DOCX duplicados) se eliminaron el 2026-11-13 para evitar thrash.

---

## 8. Post-v1 (visión, no bloquea GA)

| Fase | Contenido |
|---|---|
| v1.1 | Sandboxes / micro-VM |
| v1.2 | PR previews, CLI/SDK ricos, managed DB completo |
| v1.3 | Multi-nodo, WireGuard, certs centralizados |
| v1.4 | Wasm workers, OIDC/2FA, cuotas, HA Postgres |

---

*Actualizar las secciones 2–5 al cerrar cada sprint.*
