# Norix — Estado vs plan S0–S22

**Fecha:** 2026-11-13  
**Plan canónico:** [`docs/PLAN_SPRINTS_S0_S22.md`](./PLAN_SPRINTS_S0_S22.md)  
**Hito:** Beta 0.8 — **S17 cerrado** (leases); siguiente S18 autoscaling

---

## 1. Dónde estamos

| | |
|---|---|
| **Fase** | **Fase 4** (S15–S18) |
| **Sprint activo** | **S18** — Escalado vertical y autoscaling |
| **Último cerrado** | **S17** HA leases |
| **Versión** | `0.8.0-beta` |

---

## 2. Tablero S0–S22

| Sprint | Estado | Cierre |
|---|---|---|
| S0 | Parcial | — |
| S1–S16 | **Cerrado** | SPRINT_1…16 |
| S17 | **Cerrado** (leases; Patroni ops follow-up) | SPRINT_17_CIERRE |
| S18 | Abierto | — |
| S19–S22 | Pendiente | — |

---

## 3. Smoke local

`curl -s http://127.0.0.1:8080/healthz` → 200.
