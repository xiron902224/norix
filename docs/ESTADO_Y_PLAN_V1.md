# Norix — Estado vs plan S0–S22

**Fecha:** 2026-11-13  
**Plan canónico (23 sprints):** [`docs/PLAN_SPRINTS_S0_S22.md`](./PLAN_SPRINTS_S0_S22.md) — **se mantiene S0…S22 hasta GA 1.0.**  
**Relacionados:** `docs/adr/`, `docs/SPRINT_*_CIERRE.md`, `docs/runbooks/`, `docs/AGENT_PRODUCTION_CHECKLIST.md`, `docs/ARQUITECTURA_Y_BUENAS_PRACTICAS.md`

---

## 1. Dónde estamos

| | |
|---|---|
| **Fase actual** | **Fase 2** (multi-nodo / operación) |
| **Sprint activo (siguiente a completar)** | **S9 · Observabilidad y dashboard en vivo** |
| **Último sprint cerrado** | **S8 · Red entre nodos** (config/artifacts; apply vivo opt-in) |
| **Último hito alcanzado** | **Alpha v0.1 (S6)**; S7 scheduler + S8 mesh config |
| **Hito siguiente** | **Beta privada v0.5 (S10)** |

El recorte “S8 hardening → S10 GA” fue un atajo documental. **Queda anulado como plan de numeración.** Hardening de producción vive en **S19–S22**; no sustituye S7–S18.

Hay **scaffolds adelantados** (S9–S18) en código: cuentan como progreso técnico, **no** como sprint cerrado.

---

## 2. Tablero S0–S22 (código × plan)

Leyenda: **Cerrado** · **Parcial** · **Scaffold** · **Pendiente**

### Fase 0

| Sprint | Objetivo | Estado | Notas |
|---|---|---|---|
| **S0** | Monorepo, CI, spikes, ADRs | **Parcial** | Monorepo+CI+ADRs 001–003; spikes KVM/Firecracker no cerrados como informe |

### Fase 1 — Alpha (S1–S6)

| Sprint | Objetivo | Estado | Evidencia |
|---|---|---|---|
| **S1** | Control plane auth/RLS/projects | **Cerrado** | `SPRINT_1_CIERRE.md`, migraciones 000001–000003 |
| **S2** | Dashboard + embed | **Cerrado** | `SPRINT_2_CIERRE.md`, `web/` + `go:embed` |
| **S3** | Agente enroll/heartbeat | **Cerrado** (mejorable) | `SPRINT_3_CIERRE.md`; mTLS/PKI 24h aún liviano → reforzar en S19 / checklist agente |
| **S4** | Contenedores reconcile | **Cerrado** | `SPRINT_4_CIERRE.md`; self-heal local runtime 2026-11-13 |
| **S5** | Git/build/registry | **Cerrado** | `SPRINT_5_CIERRE.md`, zot opcional |
| **S6** | Edge/TLS/secretos — Alpha 0.1 | **Cerrado** (gaps) | `SPRINT_6_CIERRE.md`; retry/resync edge 2026-11-13; LE prod + install firmado → completar en S6 residual o S19 |

**Alpha interna: alcanzada.** No reabrir S1–S5 salvo regresiones.

### Fase 2 — Beta privada (S7–S10) ← **en curso**

| Sprint | Objetivo | Estado | Qué falta para cerrar |
|---|---|---|---|
| **S7** | Scheduler multi-nodo | **Cerrado** | `SPRINT_7_CIERRE.md`; orphan reschedule fix; stale 30s; tests scheduler |
| **S8** | Red entre nodos | **Cerrado** (apply vivo opt-in) | `SPRINT_8_CIERRE.md`; WG/DNS/nft/wildcard + tests |
| **S9** | Observabilidad en vivo | **Parcial / scaffold** | Ops S7 (métricas/alertas/backup logs) + `000013` + `/metrics`. Falta: gráficas ricas, ingest/búsqueda de logs a escala, OTel completo, retención/segmentos |
| **S10** | DB 1-click + backups — Beta 0.5 | **Parcial / scaffold** | Backups Postgres plataforma hechos; `000014` managed DBs UI. Falta: provision real multi-motor, backup a S3 cifrado, restore 1-click, **5–10 beta users** |

### Fase 3 — Sandboxes (S11–S14)

| Sprint | Objetivo | Estado |
|---|---|---|
| **S11** | Runtime micro-VM + guest agent | **Scaffold / pendiente** (`images/` stub; sin CH productivo) |
| **S12** | Snapshots, pools, API sandbox | **Scaffold** (`000015`, API/UI Sandboxes) |
| **S13** | SDKs, CLI, code interpreter | **Scaffold** (`cli/`, `sdk/` mínimos) |
| **S14** | PR previews aislados — Beta 0.8 | **Scaffold** (`000016`, API/UI Previews) |

### Fase 4 — Wasm / empresa / HA (S15–S18)

| Sprint | Objetivo | Estado |
|---|---|---|
| **S15** | Wasm first-class | **Scaffold** (`000017`, agent `wasm.rs`) |
| **S16** | Teams, cuotas, SSO/2FA | **Scaffold** (`000018`, UI Teams; SSO/TOTP stubs) |
| **S17** | HA control plane | **Scaffold** (`000019` leases; sin Patroni/multi-réplica) |
| **S18** | Autoscaling / hotplug | **Pendiente** |

### Fase 5 — Endurecimiento y GA (S19–S22)

| Sprint | Objetivo | Estado |
|---|---|---|
| **S19** | Seguridad y supply chain | **Pendiente** (+ usar `AGENT_PRODUCTION_CHECKLIST.md`) |
| **S20** | Carga, caos, SLOs | **Pendiente** |
| **S21** | Docs, upgrades, beta pública — RC 0.95 | **Pendiente** |
| **S22** | GA 1.0 | **Pendiente** |

---

## 3. Paso a paso desde ahora (sin saltar numeración)

Trabajar **en orden de fase**. Se puede picar scaffold del futuro, pero el DoD que importa es el del sprint activo.

### Ahora → cerrar Fase 2

1. **S9 — Observabilidad** ← **activo**  
   - Consolidar métricas/logs/alertas al DoD S9 (ops Alpha ya en runbook).  
   - Prometheus/OTel export estable; UI en vivo.  
   - `SPRINT_9_CIERRE.md`.  
2. **S10 — Managed DBs + backups usuario + beta**  
   - Provision Postgres/Redis (mínimo) con secretos.  
   - Backup/restore de esas DBs; invites beta.  
   - **Hito Beta v0.5** + `SPRINT_10_CIERRE.md`.

### Luego Fase 3 (S11–S14)

5. S11 micro-VM real → S12 API sandbox dura → S13 CLI/SDK → S14 PR previews.  
6. **Hito Beta sandboxes v0.8** al cerrar S14.

### Luego Fase 4 (S15–S18)

7. S15 Wasm productivo → S16 teams/SSO/cuotas → S17 HA → S18 autoscaling.

### Luego Fase 5 (S19–S22) — producción

8. **S19** threat model, SBOM, firmas, pentest, checklist agente.  
9. **S20** carga/caos/SLOs.  
10. **S21** docs + upgrades N-1 → RC 0.95.  
11. **S22** freeze API, tag **`v1.0.0`**, anuncio — **GA**.

---

## 4. Aclaraciones (para no volver a comprimir el plan)

| Idea incorrecta | Regla correcta |
|---|---|
| “Ya hicimos S7 ops, luego S8 hardening y GA” | Ops ricas = **S9**; backups/DB producto = **S10**; hardening GA = **S19–S22** |
| “Wasm/teams en el árbol ⇒ esos sprints cerrados” | Solo **scaffold** hasta cumplir aceptación del plan |
| “v1.0 = solo un nodo endurecido” | **GA 1.0 del plan original es S22** (incluye fases 2–5). Un “MVP instalable” ya existe como **Alpha S6**; no renombrar eso a GA |

Si en el futuro se recorta el producto (GA mínima), debe ser un **ADR explícito** que modifique este plan — no un silencio en el README.

---

## 5. Smoke local (Alpha ya disponible)

```bash
docker compose --env-file .env -f deploy/norix/docker-compose.yml up -d postgres pebble caddy
cd control-plane && set -a && . ./.env && set +a && go run ./cmd/server
cd web && npm run dev -- --host 127.0.0.1 --port 5173
```

| Check | Esperado |
|---|---|
| `curl -s http://127.0.0.1:8080/healthz` | 200 |
| Deploy imagen / git | contenedor `norix-*` Up |
| Dominio `*.localhost:8443` | 200 (cert interno) |

---

## 6. Mapa de docs

| Doc | Rol |
|---|---|
| **`PLAN_SPRINTS_S0_S22.md`** | Roadmap canónico S0–S22 |
| **Este archivo** | Estado real + sprint activo |
| `SPRINT_*_CIERRE.md` | Evidencia de cierre |
| `AGENT_PRODUCTION_CHECKLIST.md` | Trabajo de calidad del agente (encaja sobre todo en S3 residual + **S19**) |
| `adr/*` | Decisiones |

---

*Al cerrar un sprint: actualizar la tabla de la §2 y crear/ajustar `SPRINT_N_CIERRE.md`. No renumerar.*
