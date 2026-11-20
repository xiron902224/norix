# Norix load tests (k6)

| Script | Purpose | Requires |
|---|---|---|
| `k6-smoke.js` | `GET /healthz` only | Any HTTP 200 on `/healthz` |
| `k6-api.js` | Login, create project/workload, API reads, heartbeat fan-out | Control plane + `NORIX_LOAD_FULL=1` |

See [docs/user/load-tests.md](../../docs/user/load-tests.md).
