# Control plane

The control plane is the server-side core of Norix. It exposes the Connect RPC API, owns the PostgreSQL database, performs migrations, and enforces auth and RLS.

## Responsibilities

- bootstrap and first admin setup
- login and logout
- session validation and token hashing
- RLS-aware DB access
- project CRUD and work items metadata
- health checks and lifecycle management

## Local run

Create the environment file:

```bash
cp .env.example .env
```

Then edit the values to match your local PostgreSQL deployment. For example:

```dotenv
DATABASE_URL=postgres://norix_app:change-me-local@127.0.0.1:5432/norix?sslmode=disable
MIGRATION_DATABASE_URL=postgres://norix:change-me-local@127.0.0.1:5432/norix?sslmode=disable
HTTP_ADDR=:8080
```

Start PostgreSQL manually or use Docker Compose from the repo root:

```bash
cd ..
docker compose --env-file .env -f deploy/norix/docker-compose.yml up -d postgres
```

Then run the control plane:

```bash
cd control-plane
go run ./cmd/server
```

Or use migration-only mode in CI or release scripts:

```bash
DATABASE_URL=postgres://norix_app:change-me-local@127.0.0.1:5432/norix?sslmode=disable \
  go run ./cmd/server -migrate-only
```

## Health check and embedded UI

```bash
curl http://127.0.0.1:8080/healthz
# ok

# SPA (after make embed-web / Docker build)
curl -I http://127.0.0.1:8080/
```

Build the UI into the binary from the repo root with `make build`.

## Important security notes

- The server stores only hashes of tokens and password hashes.
- The public login and bootstrap endpoints are rate limited.
- PostgreSQL RLS is expected to isolate tenant data.
- The server does not accept a JWT as trusted input; it uses the session table and current token hash as the source of truth.
