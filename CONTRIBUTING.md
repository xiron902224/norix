# Contributing to Norix

Thanks for helping. Norix is Apache-2.0 open source. Small, reviewable changes are preferred.

**Español:** [`CONTRIBUTING.es.md`](CONTRIBUTING.es.md) · [`README.es.md`](README.es.md)

Production install (VPS): see the [README Install](README.md#install) section (`--mode docker` or `--mode bare`). This file is for **developers** working on the monorepo.

**What to work on:** [`ISSUES.md`](ISSUES.md) (triage) or [open GitHub issues](https://github.com/xiron902224/norix/issues) — prefer **P0** / **good first issue** unless you already own a larger slice.

## Gitflow (main only)

| Branch | Role |
|---|---|
| `main` | Default branch · integration tip · **only** PR target |
| `feature/*`, `fix/*`, `docs/*`, `chore/*` | Short-lived work branched **from `main`** |
| `v*` tags | Releases (binaries + SHA256SUMS + cosign) |

`develop` is **retired** as an integration branch (historical only; do not open new PRs to it).

### Workflow

1. Pick a [GitHub issue](https://github.com/xiron902224/norix/issues) (comment that you are taking it).
2. Branch from latest `main`:
   ```bash
   git fetch origin && git checkout main && git pull
   git checkout -b feature/123-short-slug   # or fix/… docs/… chore/…
   ```
3. Implement, test, commit.
4. Open a PR **to `main`**. Link the issue (`Fixes #123` / `Refs #123`).
5. Wait for CI. Maintainers merge when green (+ required review).

```text
issue → branch from main → PR → main → (optional) tag v*
```

Rules of thumb:

1. **Always** PR to `main` (not `develop`).
2. Tag releases from a green `main` only: `v1.0.0-beta.3`, later `v1.0.0`.
3. Pushing a `v*` tag runs `.github/workflows/release.yml`.
4. Protobuf breaking changes are checked in CI against the PR **base** (`main`).

## Local development

For day-to-day coding on a laptop (not the production installer):

```bash
git clone https://github.com/xiron902224/norix.git && cd norix
git checkout main

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
buf breaking --against ".git#ref=origin/main"
```

## Pull requests

1. Branch from latest `main`.
2. Keep the diff focused; update `docs/ESTADO_Y_PLAN_V1.md` / `ISSUES.md` only when status meaningfully changes.
3. Target **`main`**. Wait for CI (protos → DB/RLS + web + agent → control-plane).
4. Fill the GitHub PR template (`.github/PULL_REQUEST_TEMPLATE.md`) — do not leave placeholders empty.
5. Link the GitHub issue in the body (`Fixes #N` / `Refs #N`).

### PR title format (required for review)

```text
type(#issue): short imperative summary
```

| `type` | Use when |
|---|---|
| `fix` | Bug fix |
| `feat` | New capability or user-visible enhancement |
| `docs` | Docs / templates only |
| `chore` | CI, release, deps, repo hygiene |
| `refactor` | Internal cleanup, no intended behavior change |

**Examples (mergeable titles):**
- `fix(#71): show real control-plane and agent versions`
- `feat(#73): add norix-control-plane update command`
- `docs: add pull request template`
- `chore(#62): publish v1.0.0-beta.3 release assets`
- `fix(#59): map DeployFromGit unschedulable to FailedPrecondition`

Avoid: vague titles (`update`, `fixes`, `wip`), file dumps (`edit main.go and NodesPage.tsx`), or missing issue when one exists.

## Code style

- Match neighboring code; no drive-by refactors.
- Go: standard library + existing packages under `control-plane/internal/`.
- Web: existing React/Vite patterns under `web/src/`.
- Agent: Rust 1.88 toolchain as in CI.
- After proto edits: `make proto-generate` and commit generated Go/TS/Rust clients.

## Docs

- Living status: [`docs/ESTADO_Y_PLAN_V1.md`](docs/ESTADO_Y_PLAN_V1.md)
- Vision / maturity: [`VISION.md`](VISION.md)
- Backlog triage: [`ISSUES.md`](ISSUES.md)
- Bare install: [`docs/runbooks/BARE_INSTALL.md`](docs/runbooks/BARE_INSTALL.md)
- Security reports: [`SECURITY.md`](SECURITY.md) (private, not public issues)
- Spanish: [`README.es.md`](README.es.md), [`CONTRIBUTING.es.md`](CONTRIBUTING.es.md)

## License

By contributing, you agree your contributions are licensed under the [Apache License 2.0](LICENSE).
