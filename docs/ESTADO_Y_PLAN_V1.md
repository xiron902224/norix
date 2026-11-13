# Norix — Estado vs plan S0–S22

**Fecha:** 2026-11-13  
**Plan canónico:** [`docs/PLAN_SPRINTS_S0_S22.md`](./PLAN_SPRINTS_S0_S22.md)  
**Hito:** Fase 5 — **S20 cerrado**; siguiente S21 RC docs

---

## 1. Dónde estamos

| | |
|---|---|
| **Fase** | **Fase 5** (S19–S22) |
| **Sprint activo** | **S21** — Documentación y RC v0.95 |
| **Último cerrado** | **S20** k6 + chaos catalog |
| **Versión** | `0.9.0-rc` |

---

## 2. Tablero S0–S22

| Sprint | Estado | Cierre |
|---|---|---|
| S0 | Parcial | — |
| S1–S19 | **Cerrado** | SPRINT_1…19 |
| S20 | **Cerrado** | SPRINT_20_CIERRE |
| S21 | Abierto | — |
| S22 | Pendiente | — |

---

## 3. Smoke local

`curl -s http://127.0.0.1:8080/healthz` → 200. Optional: `k6 run test/load/k6-smoke.js`.
