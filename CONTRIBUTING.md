# Contributing to Norix

Thanks for helping. Norix is Apache-2.0 open source. Small, reviewable changes are preferred.

**Español:** [`CONTRIBUTING.es.md`](CONTRIBUTING.es.md) · [`README.es.md`](README.es.md)  
**Doc map:** [`docs/README.md`](docs/README.md)

Production install (VPS): [README Install](README.md#install). This file is for **developers** working in the monorepo.

**What to work on:** [`ISSUES.md`](ISSUES.md) or [GitHub Issues](https://github.com/xiron902224/norix/issues) — prefer **P0** / **good first issue**.

## Prerequisites (local)

| Tool | Notes |
|---|---|
| Git | Clone from GitHub |
| Docker | Postgres (and optional full stack) |
| Go | Match CI (see `.github/workflows/ci.yml` / `go.mod`) |
| Node.js 22 + npm | Dashboard |
| Rust 1.88 | Agent (`rustup toolchain install 1.88.0`) |
| `buf` (optional) | Proto lint/generate — required if you touch `proto/` |
| `curl` | Smoke checks |

## Gitflow (main only)

| Branch | Role |
|---|---|
| `main` | Default · **only** PR target |
| `feature/*`, `fix/*`, `docs/*`, `chore/*` | Short-lived, branched **from `main`** |
| `v*` tags | Releases |

```text
issue → branch from main → PR → main → (optional) tag v*
```

1. Comment on the issue that you are taking it.
2. `git fetch origin && git checkout main && git pull`
3. `git checkout -b docs/123-slug` (or `fix/` / `feat/` / `chore/`)
4. Implement + test locally.
5. Open a PR **to `main`**, fill [`.github/PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md), link `Fixes #N`.

## Local development (full path)

### 1. Clone and env

```bash
git clone https://github.com/xiron902224/norix.git && cd norix
git checkout main

cp .env.example .env
cp control-plane/.env.example control-plane/.env
cp web/.env.example web/.env
# Optional: echo 'NORIX_VERSION=1.0.0-beta.4' >> control-plane/.env
```

### 2. Postgres

```bash
docker compose --env-file .env -f deploy/norix/docker-compose.yml up -d postgres
```

### 3. Control plane

```bash
cd control-plane && set -a && . ./.env && set +a && go run ./cmd/server
# → http://127.0.0.1:8080/healthz  and  /status
```

### 4. Dashboard (second terminal)

```bash
cd web && npm install && npm run dev -- --host 127.0.0.1 --port 5173
# → http://127.0.0.1:5173  (API via Vite proxy / CORS)
```

Open the UI → create the **first admin** → create a project.

### 5. Agent on the same machine (optional, for real deploys)

Needs Docker socket on the host.

```bash
# In the UI: Nodes → Join token
cd agent && cargo build --release
export NORIX_CONTROL_PLANE_URL=http://127.0.0.1:8080
export NORIX_JOIN_TOKEN='…'          # one-time token from UI
export NORIX_NODE_NAME="$(hostname -s)"
export NORIX_AGENT_WORKDIR=/tmp/norix-agent-dev
./target/release/norix-agent
```

Or use [`scripts/install-agent.sh`](scripts/install-agent.sh) with `--supervise none` / `--foreground`.

Confirm **Nodes** shows the agent `online` with `dockerSocket: true` before Git builds.

### 6. Single binary (embedded UI)

```bash
make build
./control-plane/bin/norix-control-plane
# → http://127.0.0.1:8080
```

| Service | URL |
|---|---|
| Dashboard (Vite) | http://127.0.0.1:5173 |
| Control plane | http://127.0.0.1:8080/healthz · `/status` |
| Postgres | `127.0.0.1:5432` |

Do not commit `.env`, join tokens, or `NORIX_BUILD_SECRETS_KEY`.

## Test loop (what CI runs)

```bash
# API
cd control-plane && go test ./...

# UI
cd web && npm ci && npm run lint && npm run build

# Agent
cd agent && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test

# Protos (if you change proto/)
make proto-lint
buf breaking --against ".git#ref=origin/main"
```

Quick smoke after services are up:

```bash
curl -sf http://127.0.0.1:8080/healthz
curl -sf http://127.0.0.1:8080/status | jq .
```

### Required status checks on `main`

Branch protection requires these job **`name:`** values from [`.github/workflows/ci.yml`](.github/workflows/ci.yml) — keep them in sync if you rename jobs:

| Required check |
|---|
| Validate Protobuf Contracts |
| PostgreSQL migrations and RLS |
| Build Web Dashboard |
| Build Rust Agent |
| Security vulnerability scans |
| Build Control Plane (Go) |

Do **not** rename jobs without updating branch protection (Settings → Branches → `main`).

## Pull requests

1. Branch from `main`; target **`main`**.
2. Fill the PR template (Summary, Issue, Test plan, **Cómo probarlo**).
3. Title format: `type(#issue): short imperative summary`  
   (`fix` · `feat` · `docs` · `chore` · `refactor`)
4. Wait for CI (protos → DB/RLS + web + agent + security → control-plane).

Examples: `fix(#59): …` · `docs(#66): …` · `feat(#73): …`

## Collaborators & contributors (GitHub)

**Contributors** (anyone with commits/PRs merged) appear automatically on:

- Repo Insights → Contributors  
- The [Contributors](README.md#contributors) section in the README  

**Collaborators** (write access to the repo) are managed by maintainers:

1. GitHub → **Settings** → **Collaborators** → **Add people**
2. Invite by username (role: Write for regular contributors)
3. They must **accept** the email/invitation  

Being a collaborator ≠ automatic README credit; merged PRs create contributor credit. Maintainers may also list people under Collaborators in the README.

## Docs

- Map: [`docs/README.md`](docs/README.md)
- Status / vision / backlog: [`docs/ESTADO_Y_PLAN_V1.md`](docs/ESTADO_Y_PLAN_V1.md) · [`VISION.md`](VISION.md) · [`ISSUES.md`](ISSUES.md)
- Ops: [`docs/runbooks/`](docs/runbooks/)
- Security: [`SECURITY.md`](SECURITY.md) (private reports)

## License

By contributing, you agree your contributions are licensed under the [Apache License 2.0](LICENSE).
