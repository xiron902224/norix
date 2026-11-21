# Runtime Wasm (GA) (#47)

Registra funciones en **Runtimes**. El CP envía `DesiredWasmPlan`; el agent materializa `.wasm` y lo ejecuta con **Wasmtime** (fuel + memoria).

## Materialización

Rutas locales / HTTP, o OCI vía `oras` → `docker cp` → `docker export`. Sin stub `.ref` silencioso.

## Serve y edge

`wasmtime serve` por defecto (`NORIX_WASM_SERVE=0` para desactivar). Puerto estable 19000–19999 + hostname; el edge apunta Caddy a ese upstream.

Guía EN: [wasm.md](../wasm.md).
