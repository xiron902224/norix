# Sprint 15 — WebAssembly de primera clase

## Estado
**Cerrado 2026-11-13** (limits + API/UI). **Agent path v1.0 (2026-11-14):** `DesiredWasm` / `ReportWasm` en heartbeat, staging de artefactos + smoke `wasmtime` opcional.

## Entregables
- `internal/wasm` fuel/memory/timeout limits + cold-start budget 5ms
- WasmService + agent wasm reconciler (`agent/src/wasm.rs`)
- Proto: `DesiredWasmPlan`, `ReportWasm`

## Tests
`go test ./internal/wasm/ -count=1` · `cargo test --release` en `agent/`
