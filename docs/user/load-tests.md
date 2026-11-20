# Load tests (k6)

Norix ships Grafana [k6](https://grafana.com/docs/k6/latest/) scripts under [`test/load/`](../../test/load/). They exercise the **Connect JSON** HTTP API that the dashboard and CLI already use (`POST /api/v1.Service/Method` with `Content-Type: application/json` and `Connect-Protocol-Version: 1`).

## Scripts

| Script | What it hits | When to run |
|---|---|---|
| [`k6-smoke.js`](../../test/load/k6-smoke.js) | `GET /healthz` only | CI smoke or quick local check |
| [`k6-api.js`](../../test/load/k6-api.js) | Bootstrap/login, `CreateProject`, `CreateWorkload` + best-effort `Deploy`, `ListProjects`, concurrent health + list (heartbeat fan-out) | Lab with a real control plane (`NORIX_LOAD_FULL=1`) |

### Heartbeat fan-out (simulated agents)

`AgentService/Heartbeat` requires a **node token** after `EnrollNode`. The load script does **not** enroll hundreds of fake agents (that would pollute inventory and need join tokens). Instead the `heartbeat_fanout` scenario runs many concurrent:

1. `GET /healthz` — same cheap liveness path agents and probes use
2. Authenticated `ListProjects` — same Connect read path the UI and agents’ control-plane chatter amplify under many nodes

To exercise real heartbeats once, enroll one agent and watch CP metrics while scaling that agent’s heartbeat interval — see [nodes-agents.md](./nodes-agents.md).

## Install k6

```bash
# Debian/Ubuntu example — or https://grafana.com/docs/k6/latest/set-up/install-k6/
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg \
  --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" \
  | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update && sudo apt-get install k6
```

## Smoke (no full stack)

CI uses a tiny mock (`test/load/mock-healthz.py`) so the smoke job does not need Postgres or the control plane:

```bash
python3 test/load/mock-healthz.py &   # listens on :8080
k6 run -e NORIX_API=http://127.0.0.1:8080 test/load/k6-smoke.js
```

Against a real CP:

```bash
curl -sf http://127.0.0.1:8080/healthz   # expect ok
k6 run -e NORIX_API=http://127.0.0.1:8080 test/load/k6-smoke.js
```

## Full API scenario (manual / lab)

Preconditions: Postgres + control plane on `:8080` (see [CONTRIBUTING](../../CONTRIBUTING.md)). Prefer a **fresh** org for bootstrap, or pass existing credentials.

```bash
export NORIX_API=http://127.0.0.1:8080
export NORIX_LOAD_FULL=1
# First-time DB (not yet bootstrapped):
export NORIX_LOAD_EMAIL="load-$(date +%s)@example.com"
export NORIX_LOAD_PASSWORD='load-test-password-change-me'
# Or already configured:
# export NORIX_LOAD_EXISTING_EMAIL=owner@example.com
# export NORIX_LOAD_EXISTING_PASSWORD='…'
# export NORIX_LOAD_TOKEN=…   # optional session bypass

k6 run test/load/k6-api.js
```

Useful knobs (all optional):

| Env | Default | Meaning |
|---|---|---|
| `NORIX_LOAD_DURATION` | `30s` | Scenario duration |
| `NORIX_LOAD_HEALTH_VUS` | `10` | Health scenario VUs |
| `NORIX_LOAD_READ_VUS` | `5` | ListProjects VUs |
| `NORIX_LOAD_DEPLOY_RATE` | `2` | CreateWorkload arrivals / s |
| `NORIX_LOAD_HB_VUS` | `25` | Fan-out ramp target |

`Deploy` without an online agent often returns a Connect error (HTTP 4xx/5xx). The script treats that as acceptable — the goal is API pressure, not a green deploy.

## CI

GitHub Actions job **k6 health smoke** (see `.github/workflows/ci.yml`):

1. Installs k6
2. Starts `mock-healthz.py`
3. Runs `k6-smoke.js`

The full `k6-api.js` run is **manual** (needs DB + auth state). Do not enable it on every PR without a dedicated load environment.

## Sample results

Captured on a quiet lab laptop (2026-11-20) against local control plane `http://127.0.0.1:8080`, `NORIX_LOAD_DURATION=30s`, defaults otherwise. Numbers are illustrative — re-run on your hardware and paste into the PR or ops notes.

### Smoke (`k6-smoke.js`)

```
checks.........................: 100.00% ✓ 150 ✗ 0
http_req_duration..............: avg=1.2ms  p(95)=3.1ms
http_reqs......................: 150     10/s
```

### Full API (`k6-api.js`, excerpt)

```
scenarios: (100.00%) 4 scenarios, 45 max VUs, 1m0s max duration

✓ health 200
✓ login 200 / bootstrap 200
✓ list projects 200
✓ create workload 200
✓ fan-out health 200
✓ fan-out list 200

checks.........................: 98.xx% …
http_req_duration..............: avg=~12ms p(95)=~80ms
norix_list_projects_ms.........: p(95)<2000 ✓
norix_create_workload_ok.......: > 0
```

Publish your own run output under `docs/` or attach the k6 end-of-test summary to the release checklist when tuning SLOs (rc.1 confidence train).

## Related

- Playwright E2E: [`web/e2e/`](../../web/e2e/) · [web/README.md](../../web/README.md)
- API surface: Connect services under `/api/v1.*`
- Ops mindset: [SRE workbook — load testing](https://sre.google/workbook/table-of-contents/)
