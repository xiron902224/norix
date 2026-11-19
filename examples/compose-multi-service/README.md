# Multi-service Compose example

Three services in one stack: `db` (Postgres), `api` (Node), and `web` (nginx). The stack shows the Compose features that Norix adds on top of `docker compose`:

| Service | Norix setting | Why |
|---|---|---|
| `db` | internal (default) | Reachable only on the project network as `db` or `norix-<id>-db`. |
| `api` | internal (default) | Its `/` endpoint checks the TCP connection to `db` (#144). |
| `web` | public | Keeps host port `8081` as written in the compose file (#146). |

## Deploy in Norix

1. Project → **Compose stack** → Git URL of this repo, ref `main`, compose file `examples/compose-multi-service/docker-compose.yml`.
2. **Public services**: `web`.
3. **Variables**: `DB_PASSWORD=<a value>`. The apply fails with a clear message if it is missing (#145).
4. Optional: **Domains** `web=app.example.com`, so Caddy routes HTTPS to `web` on the same node.

## Run locally

```bash
DB_PASSWORD=dev-only docker compose -f examples/compose-multi-service/docker-compose.yml up -d --build --wait
curl -s http://127.0.0.1:8081/api/     # {"service":"api","db":"reachable"}
docker compose -f examples/compose-multi-service/docker-compose.yml down -v
```

CI runs the same steps in the `Compose example (#35)` job.
