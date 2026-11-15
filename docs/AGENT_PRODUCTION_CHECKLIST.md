# Norix Agent — Checklist de producción

**Estado Beta 1 (2026-11-15):** DoD de código cerrado (contenedor + wasm staging + micro-VM gated).  
Pendiente ops: E2E agent↔CP y tag `v1.0.0-beta.4` tras merge (GA después; ver `docs/ESTADO_Y_PLAN_V1.md` §4 y `ISSUES.md`).

## DoD cumplido

- [x] Validar `NORIX_CONTROL_PLANE_URL` / token / workdir
- [x] Modo `--check`
- [x] Enroll + `node-state.json`
- [x] Heartbeat con desired workloads/builds/edge/network/wasm
- [x] Backoff en fallos de heartbeat
- [x] Reconciliación contenedores (labels, generation, cpu-shares, memory)
- [x] `running_workload_ids` en heartbeat
- [x] DesiredWasm + ReportWasm + opcional `wasmtime serve` (`NORIX_WASM_SERVE=1`)
- [x] Micro-VM Cloud Hypervisor cuando hay KVM + kernel/rootfs
- [x] Mesh apply opt-in (`NORIX_APPLY_WIREGUARD=1` en CP)
- [x] Modo degraded si runtime no disponible

## Smoke E2E

```bash
cd agent && cargo build --release
./target/release/norix-agent --check
# Con CP arriba y join token:
NORIX_CONTROL_PLANE_URL=http://127.0.0.1:8080 NORIX_JOIN_TOKEN=... ./target/release/norix-agent
```
