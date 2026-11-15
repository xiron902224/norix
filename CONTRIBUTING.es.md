# Contribuir a Norix

Gracias por ayudar. Norix es open source bajo Apache-2.0. Preferimos cambios pequeños y fáciles de revisar.

**English:** [`CONTRIBUTING.md`](CONTRIBUTING.md) · [`README.md`](README.md)

Instalación en VPS: ver [Instalar](README.es.md#instalar) en el README en español (`--mode docker` o `--mode bare`). Este archivo es para quien **desarrolla** en el monorepo.

**En qué trabajar:** [`ISSUES.md`](ISSUES.md) o [issues en GitHub](https://github.com/xiron902224/norix/issues) — prioriza **P0** / **good first issue** salvo que ya tengas un bloque más grande asignado.

## Gitflow (solo `main`)

| Rama | Rol |
|---|---|
| `main` | Rama por defecto · tip de integración · **único** destino de PRs |
| `feature/*`, `fix/*`, `docs/*`, `chore/*` | Trabajo corto **partiendo de `main`** |
| tags `v*` | Releases (binarios + SHA256SUMS + cosign) |

`develop` queda **retirada** como rama de integración (solo histórico; no abras PRs nuevos hacia ella).

### Flujo de trabajo

1. Elige un [issue de GitHub](https://github.com/xiron902224/norix/issues) (comenta que lo tomas).
2. Crea la rama desde `main` actualizado:
   ```bash
   git fetch origin && git checkout main && git pull
   git checkout -b feature/123-slug-corto   # o fix/… docs/… chore/…
   ```
3. Implementa, prueba y haz commit.
4. Abre un PR **hacia `main`**. Enlaza el issue (`Fixes #123` / `Refs #123`).
5. Espera CI. Los maintainers mergean con CI verde (+ review requerida).

```text
issue → rama desde main → PR → main → (opcional) tag v*
```

Reglas rápidas:

1. **Siempre** PR a `main` (no a `develop`).
2. Tags solo desde un `main` verde: `v1.0.0-beta.4`, luego `v1.0.0`.
3. Empujar un tag `v*` dispara `.github/workflows/release.yml`.
4. Compatibilidad protobuf en CI vs la base del PR (`main`).

## Desarrollo local

```bash
git clone https://github.com/xiron902224/norix.git && cd norix
git checkout main

cp .env.example .env
cp control-plane/.env.example control-plane/.env
cp web/.env.example web/.env

docker compose --env-file .env -f deploy/norix/docker-compose.yml up -d postgres

cd control-plane && set -a && . ./.env && set +a && go run ./cmd/server

# otra terminal
cd web && npm install && npm run dev -- --host 127.0.0.1 --port 5173
```

| Servicio | URL |
|---|---|
| Dashboard (Vite) | http://127.0.0.1:5173 |
| Control plane | http://127.0.0.1:8080/healthz |
| Postgres | `127.0.0.1:5432` |

**Binario único (UI embebida):**

```bash
make build
./control-plane/bin/norix-control-plane
```

No subas `.env`, join tokens ni `NORIX_BUILD_SECRETS_KEY`.

## Bucle de pruebas

```bash
cd control-plane && go test ./...
cd web && npm ci && npm run lint && npm run build
cd agent && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test
make proto-lint
buf breaking --against ".git#ref=origin/main"
```

## Pull requests

1. Rama desde `main`.
2. Diff enfocado; actualiza `docs/ESTADO_Y_PLAN_V1.md` / `ISSUES.md` solo si el estado cambia de verdad.
3. Destino **`main`**. CI: protos → DB/RLS + web + agent → control-plane.
4. Rellena la plantilla de PR (`.github/PULL_REQUEST_TEMPLATE.md`) — no dejes placeholders vacíos.
5. Enlaza el issue (`Fixes #N` / `Refs #N`).

### Formato del título (obligatorio para revisar)

```text
type(#issue): resumen corto en imperativo
```

| `type` | Cuándo |
|---|---|
| `fix` | Bug |
| `feat` | Feature / mejora visible |
| `docs` | Solo documentación |
| `chore` | CI, release, deps |
| `refactor` | Limpieza interna |

**Ejemplos:** `fix(#71): show real control-plane and agent versions` · `feat(#73): add norix-control-plane update` · `docs: add pull request template`

## Estilo de código

- Sigue el código vecino; sin refactors laterales.
- Go: stdlib + paquetes en `control-plane/internal/`.
- Web: patrones React/Vite en `web/src/`.
- Agent: toolchain Rust 1.88 como en CI.
- Tras editar protos: `make proto-generate` y commit de clientes generados.

## Docs

- Estado: [`docs/ESTADO_Y_PLAN_V1.md`](docs/ESTADO_Y_PLAN_V1.md)
- Visión: [`VISION.md`](VISION.md)
- Backlog: [`ISSUES.md`](ISSUES.md)
- Bare: [`docs/runbooks/BARE_INSTALL.md`](docs/runbooks/BARE_INSTALL.md)
- Seguridad: [`SECURITY.md`](SECURITY.md) (privado)
- Español: este archivo + [`README.es.md`](README.es.md)

## Licencia

Al contribuir, aceptas que tus aportes se licencian bajo [Apache License 2.0](LICENSE).
