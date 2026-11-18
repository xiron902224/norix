# ADR-005 — CLI oficial en Rust

Estado: aceptado  
Issue: [#28](https://github.com/xiron902224/norix/issues/28)

## Contexto

El directorio `cli/` era un stub en **Go** (Cobra) con `deploy` vacío. El agente de nodo ya está en Rust (`agent/`). Mantener un segundo lenguaje solo para el CLI aumenta superficie de CI, release y onboarding sin beneficio claro.

## Decisión

El CLI de usuario **`norix`** se implementa en **Rust** (edition 2021):

- Parsing: [`clap`](https://docs.rs/clap) + [`clap_complete`](https://docs.rs/clap_complete)
- HTTP: `reqwest` (rustls) contra la API Connect JSON (`/api/v1.*`)
- Auth: bearer en `NORIX_TOKEN` o `~/.norix/token` (0600)
- Binario de release: `norix-cli-linux-{amd64,arm64}` (mismo nombre de asset que antes)

## Alternativas

| Opción | Por qué no |
|---|---|
| Seguir en Go (Cobra) | Duplica toolchain vs agent; el stub ya no cubría #28 |
| Generar CLI desde proto (openapi-generator) | Útil más adelante con #29; no bloquea un CLI usable ahora |
| TypeScript / Node | Mala DX para binario ops single-file en servidores |

## Consecuencias

- CI/release construyen `cli/` con Cargo (como `agent/`)
- Dependabot: ecosystem `cargo` en `/cli` (se retira `gomod` de `/cli`)
- Docs de usuario deben decir explícitamente: **CLI escrito en Rust**
- El código Go de `cli/` se elimina; no hay compatibilidad de flags con el stub más allá de `--api` / `--token` / `health` / `login` / `projects list`
