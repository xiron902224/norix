# Contribuir a Norix

Gracias por ayudar. Norix es open source bajo Apache-2.0. Preferimos cambios pequeños y fáciles de revisar.

**English:** [`CONTRIBUTING.md`](CONTRIBUTING.md) · [`README.md`](README.md)  
**Mapa de docs:** [`docs/README.md`](docs/README.md)

Instalación en VPS: [Instalar](README.es.md#instalar). Este archivo es para quien **desarrolla** en el monorepo.

**En qué trabajar:** [`ISSUES.md`](ISSUES.md) o [issues](https://github.com/xiron902224/norix/issues) — prioriza **P0** / **good first issue**.

## Requisitos (local)

| Herramienta | Notas |
|---|---|
| Git | Clone |
| Docker | Postgres |
| Go | Como en CI / `go.mod` |
| Node.js 22 + npm | Dashboard |
| Rust 1.88 | Agent + CLI (`cli/`, ADR-005) |
| `buf` (opcional) | Solo si tocas `proto/` |

## Gitflow (solo `main`)

```text
issue → rama desde main → PR → main → (opcional) tag v*
```

1. Comenta en el issue que lo tomas.
2. `git fetch origin && git checkout main && git pull`
3. `git checkout -b docs/123-slug` (o `fix/` / `feat/` / `chore/`)
4. Implementa y prueba en local.
5. PR **a `main`**, plantilla [`.github/PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md), `Fixes #N`.

## Desarrollo local (camino completo)

### 1. Clone y env

```bash
git clone https://github.com/xiron902224/norix.git && cd norix
git checkout main
cp .env.example .env
cp control-plane/.env.example control-plane/.env
cp web/.env.example web/.env
```

### 2. Postgres

```bash
docker compose --env-file .env -f deploy/norix/docker-compose.yml up -d postgres
```

### 3. Control plane

```bash
cd control-plane && set -a && . ./.env && set +a && go run ./cmd/server
```

### 4. Dashboard (otra terminal)

```bash
cd web && npm install && npm run dev -- --host 127.0.0.1 --port 5173
```

Crea el **primer admin** en la UI.

### 5. Agent (opcional, mismo host con Docker)

```bash
cd agent && cargo build --release
export NORIX_CONTROL_PLANE_URL=http://127.0.0.1:8080
export NORIX_JOIN_TOKEN='…'   # Nodes → Join token
export NORIX_NODE_NAME="$(hostname -s)"
./target/release/norix-agent
```

Antes de DeployFromGit: nodo `online` y `dockerSocket: true` (ver [BARE_INSTALL](docs/runbooks/BARE_INSTALL.md#un-worker-online-no-acepta-builds-git)).

### 6. Binario único

```bash
make build && ./control-plane/bin/norix-control-plane
```

| Servicio | URL |
|---|---|
| UI Vite | http://127.0.0.1:5173 |
| API | http://127.0.0.1:8080/healthz · `/status` |

## Pruebas (como CI)

```bash
cd control-plane && go test ./...
cd web && npm ci && npm run lint && npm run build
cd agent && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test
cd cli && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test
curl -sf http://127.0.0.1:8080/healthz
```

## Pull requests

- Destino **`main`**
- Título: `type(#issue): resumen` (`fix` · `feat` · `docs` · `chore` · `refactor`)
- Rellena la plantilla (incluye **Cómo probarlo**)

## Colaboradores y contributors

- **Contributors:** quien mergea PRs aparece en Insights y en el README ([Contributors](README.md#contributors)).
- **Collaborators (write):** el maintainer invita en GitHub → **Settings** → **Collaborators** → **Add people**. Hay que **aceptar** la invitación.

## Docs

Ver [`docs/README.md`](docs/README.md).

## Licencia

Aportes bajo [Apache License 2.0](LICENSE).
