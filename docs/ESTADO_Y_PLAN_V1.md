# Norix — Estado vs plan S0–S22

**Fecha:** 2026-11-13  
**Plan canónico:** [`docs/PLAN_SPRINTS_S0_S22.md`](./PLAN_SPRINTS_S0_S22.md)  
**Hito:** Fase 5 — **S19 cerrado**; siguiente S20 load/chaos

---

## 1. Dónde estamos

| | |
|---|---|
| **Fase** | **Fase 5** (S19–S22) |
| **Sprint activo** | **S20** — Rendimiento, carga y caos |
| **Último cerrado** | **S19** threat model + SBOM |
| **Versión** | `0.9.0-rc` |

---

## 2. Tablero S0–S22

| Sprint | Estado | Cierre |
|---|---|---|
| S0 | Parcial | — |
| S1–S18 | **Cerrado** | SPRINT_1…18 |
| S19 | **Cerrado** | SPRINT_19_CIERRE |
| S20 | Abierto | — |
| S21–S22 | Pendiente | — |

---

## 3. Smoke local

`curl -s http://127.0.0.1:8080/healthz` → 200. `./scripts/sbom.sh` escribe `dist/sbom/`.
