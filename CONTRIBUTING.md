# Contributing to Norix

Thanks for helping. Norix is Apache-2.0 open source. Small, reviewable changes against `develop` are preferred.

Production install (VPS): see the [README Install](README.md#install) section (`--mode docker` or `--mode bare`). This file is for **developers** working on the monorepo.

## Gitflow

| Branch | Role |
|---|---|
| `develop` | Integration / default target for PRs |
| `feature/*` | Short-lived work; open PR → `develop` |
| `release` | Stabilization cut when preparing a tagged release |
| `nightly` | Optional experimental / CI playground |
| `main` | Production tip **after** a release is signed off (still an early stub — do not treat as current API) |

Rules of thumb:

1. **Never** open feature PRs straight to `main`.
2. Tag releases from a green `develop` (or `release`) only: `v1.0.0-beta.1`, later `v1.0.0`.
3. Pushing a `v*` tag runs `.github/workflows/release.yml` (binaries + SHA256SUMS + cosign).
4. Protobuf breaking changes are checked in CI against the PR **base** branch.

```text
feature/* ──PR──► develop ──(stabilize)──► release ──tag v*──► GitHub Release
                                              │
                                              └──► main (fast-forward when a release is cut)
```

## Local development

For day-to-day coding on a laptop (not the production installer):

```bash
git clone https://github.com/xiron902224/norix.git && cd norix
git checkout develop

cp .env.example .env
cp control-plane/.env.example control-plane/.env
cp web/.env.example web/.env

# Postgres only
docker compose --env-file .env -f deploy/norix/docker-compose.yml up -d postgres

# Control plane (migrates DB on boot)
cd control-plane && set -a && . ./.env && set +a && go run ./cmd/server

# Dashboard (another terminal)
cd web && npm install && npm run dev -- --host 127.0.0.1 --port 5173
```

| Service | URL |
|---|---|
| Dashboard (Vite) | http://127.0.0.1:5173 |
| Control plane | http://127.0.0.1:8080/healthz |
| Postgres | `127.0.0.1:5432` |

**Single binary (embedded UI):**

```bash
make build
./control-plane/bin/norix-control-plane
# open http://127.0.0.1:8080
```

Optional edge / registry profiles and the Rust agent: see comments in `deploy/norix/docker-compose.yml` and `agent/README.md`.

Do not commit `.env`, join tokens, or `NORIX_BUILD_SECRETS_KEY`.

## Dev loop

```bash
# API
cd control-plane && go test ./...

# UI
cd web && npm ci && npm run lint && npm run build

# Agent
cd agent && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test

# Protos (requires buf)
make proto-lint
buf breaking --against ".git#ref=origin/develop"
```

## Pull requests

1. Branch from latest `develop`.
2. Keep the diff focused; update `docs/ESTADO_Y_PLAN_V1.md` only when status meaningfully changes.
3. PR title: what / why, not a dump of file names.
4. Target **`develop`**. Wait for CI (protos → DB/RLS + web + agent → control-plane).
5. Link any sprint / issue context in the body.

## Code style

- Match neighboring code; no drive-by refactors.
- Go: standard library + existing packages under `control-plane/internal/`.
- Web: existing React/Vite patterns under `web/src/`.
- Agent: Rust 1.88 toolchain as in CI.
- After proto edits: `make proto-generate` and commit generated Go/TS/Rust clients.

## Docs

- Living status: [`docs/ESTADO_Y_PLAN_V1.md`](docs/ESTADO_Y_PLAN_V1.md)
- Roadmap: [`docs/PLAN_SPRINTS_S0_S22.md`](docs/PLAN_SPRINTS_S0_S22.md)
- Bare install: [`docs/runbooks/BARE_INSTALL.md`](docs/runbooks/BARE_INSTALL.md)
- Security reports: [`SECURITY.md`](SECURITY.md) (private, not public issues)

## License

By contributing, you agree your contributions are licensed under the [Apache License 2.0](LICENSE).
