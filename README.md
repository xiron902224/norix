# Norix

Norix is a PaaS platform in early production-oriented MVP phase. The repository combines:

- control-plane: Go + Connect RPC + PostgreSQL
- web: React + Vite frontend (embedded in the control-plane binary)
- agent: Rust runtime node for enrollment, heartbeat, builds and container reconcile
- deploy: Docker Compose stack (Postgres + optional zot registry)

## Project layout

- control-plane/: Connect server, migrations, auth and DB logic
- web/: React frontend for first-run bootstrap and access control
- agent/: future runtime agent for orchestration and workers
- proto/: protobuf contracts used by Go, Rust and TypeScript
- deploy/: local deployment and Docker recipes
- docs/: architecture and sprint documentation (operational plan: `docs/ESTADO_Y_PLAN_V1.md`)

## Local stack

The project is designed to run locally with:

- PostgreSQL 16
- control-plane on port 8080
- web on Vite dev server (default 5173)
- optional agent runtime once the Rust side is implemented

## Quick start

1. Copy the environment examples:

```bash
cp .env.example .env
cp control-plane/.env.example control-plane/.env
cp web/.env.example web/.env
cp agent/.env.example agent/.env
```

2. Start PostgreSQL and the control plane:

```bash
docker compose --env-file .env -f deploy/norix/docker-compose.yml up -d --build
```

3. Start the web frontend:

```bash
cd web
npm install
npm run dev -- --host 0.0.0.0
```

4. Open the web app at http://127.0.0.1:5173

5. Open the control plane at http://127.0.0.1:8080/healthz

## Root environment variables

The root `.env` is used for Docker Compose local deployment. Example values:

```dotenv
POSTGRES_DB=norix
POSTGRES_USER=norix
POSTGRES_PASSWORD=change-me-local
POSTGRES_APP_PASSWORD=change-me-local
DATABASE_URL=postgres://norix_app:change-me-local@postgres:5432/norix?sslmode=disable
MIGRATION_DATABASE_URL=postgres://norix:change-me-local@postgres:5432/norix?sslmode=disable
HTTP_ADDR=:8080
```

## Sprint status

- **S1 closed** — auth, RLS, RBAC, projects/workloads API, audit (`docs/SPRINT_1_CIERRE.md`)
- **S2 delivered** — embedded React dashboard for projects/workloads (`docs/SPRINT_2_CIERRE.md`)
- **S3 delivered** — Rust agent enrollment, heartbeat, nodes UI (`docs/SPRINT_3_CIERRE.md`)
- **S4 delivered** — container reconcile + Deploy from dashboard (`docs/SPRINT_4_CIERRE.md`)
- **S5 delivered** — git → docker build → registry/deploy (`docs/SPRINT_5_CIERRE.md`)
- **Next: S6** — edge, domain, TLS (Alpha 0.1)

Operational plan through **v1.0**: [`docs/ESTADO_Y_PLAN_V1.md`](docs/ESTADO_Y_PLAN_V1.md)

```bash
# build UI into the Go binary, then compile
make build
./control-plane/bin/norix-control-plane
# open http://127.0.0.1:8080
```

## Important notes

- Never commit real secrets or production credentials. Keep `.env` files local only.
- The repository ignores `.env` files by default via `.gitignore`.
- The Rust agent enrolls, heartbeats, builds from git (Dockerfile), and reconciles containers (Docker/nerdctl).

## Useful commands

```bash
# local control plane tests
cd control-plane && go test ./...

# build control plane
cd control-plane && CGO_ENABLED=0 go build -o bin/norix-control-plane ./cmd/server/main.go

# web build
cd web && npm run build
```
