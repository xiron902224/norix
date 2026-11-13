# Norix — Estado vs plan S0–S22

**Fecha:** 2026-11-13  
**Plan canónico:** [`docs/PLAN_SPRINTS_S0_S22.md`](./PLAN_SPRINTS_S0_S22.md)  
**Hito:** Beta 0.5 — **S11 cerrado** (control-plane micro-VM); siguiente S12 sandboxes

---

## 1. Dónde estamos

| | |
|---|---|
| **Fase** | **Fase 3** (S11–S14 sandboxes) |
| **Sprint activo** | **S12** — Snapshots, pools y API sandboxes |
| **Último cerrado** | **S11** micro-VM runtime (KVM boot pendiente hardware) |
| **Versión** | `0.5.0-beta` |

---

## 2. Tablero S0–S22

| Sprint | Estado | Cierre |
|---|---|---|
| S0 | Parcial (spikes KVM informe) | — |
| S1–S6 | **Cerrado** (Alpha) | SPRINT_1…6 |
| S7–S10 | **Cerrado** | SPRINT_7…10 |
| S11 | **Cerrado** (control plane) | SPRINT_11_CIERRE |
| S12 | Abierto | — |
| S13–S14 | Pendiente | — |
| S15–S22 | Pendiente | — |

---

## 3. Smoke local

`curl -s http://127.0.0.1:8080/healthz` → 200.
