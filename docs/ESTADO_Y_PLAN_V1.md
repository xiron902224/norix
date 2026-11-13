# Norix — Estado vs plan S0–S22

**Fecha:** 2026-11-13  
**Plan canónico:** [`docs/PLAN_SPRINTS_S0_S22.md`](./PLAN_SPRINTS_S0_S22.md)  
**Hito:** **GA 1.0 (S22 cerrado)** — `VERSION` = 1.0.0

---

## 1. Dónde estamos

| | |
|---|---|
| **Fase** | **Fase 5 completa** (S19–S22) |
| **Sprint activo** | Post-GA: cosign multi-arch Releases + KVM micro-VM boot |
| **Último cerrado** | **S22 GA 1.0** |
| **Versión** | `1.0.0` |

---

## 2. Tablero S0–S22

| Sprint | Estado | Cierre |
|---|---|---|
| S0 | Parcial (spikes KVM informe) | — |
| S1–S6 | **Cerrado** (Alpha) | SPRINT_1…6 |
| S7–S10 | **Cerrado** (Beta 0.5) | SPRINT_7…10 |
| S11–S14 | **Cerrado** (Beta sandboxes 0.8) | SPRINT_11…14 |
| S15–S18 | **Cerrado** | SPRINT_15…18 |
| S19–S21 | **Cerrado** (RC 0.95) | SPRINT_19…21 |
| S22 | **Cerrado** (GA 1.0) | SPRINT_22_CIERRE |

---

## 3. Límites honestos de GA

Incluido: auth/RLS, deploy, edge, ops, scheduler, mesh config, observability, managed DB API, sandboxes/previews/wasm/teams surfaces, HA leases, autoscaler policy, docs/security.

Requiere ops/hardware extra: WireGuard live apply, micro-VM boot en KVM real, Patroni multi-réplica, cosign release artifacts.

---

## 4. Smoke local

Ver README Quick Start. `curl -s http://127.0.0.1:8080/healthz` → 200.
