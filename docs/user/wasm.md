# Wasm runtime (GA) (#47)

Register Wasm functions in the dashboard (**Runtimes**). The control plane ships a `DesiredWasmPlan` on heartbeats; agents materialize `.wasm` artifacts and run them under **Wasmtime** with fuel + memory limits.

## Materialization

| `image_ref` | Behavior |
|---|---|
| Local path / `http(s)://…/*.wasm` | Copy or `curl` |
| OCI ref | `oras pull` → find `.wasm`; else `docker create`+`cp` known paths; else `docker export` scan for `*.wasm` |

Failure to materialize reports `failed` (no silent `.ref` stub).

## Limits & serve

- Fuel: `fuel_limit` (default 1e8)
- Memory: `memory_limit_mb` (default 128)
- Listen: stable `listen_port` in **19000–19999** from function id; hostname `*.wasm.norix.internal` style FQDN

`wasmtime serve` starts by default when available. Opt out: `NORIX_WASM_SERVE=0` on the agent.

## Edge

Edge plans include upserts for each function’s hostname → `http://<EdgeUpstreamHost>:<listen_port>` so Caddy can route HTTP to the WASI server.

## Tests

```bash
cd agent && cargo test wasm
cd control-plane && go test ./internal/db/ -run Wasm -count=1
```
