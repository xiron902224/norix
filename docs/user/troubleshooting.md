# Troubleshooting

## Quick table

| Symptom | Likely cause | What to do |
|---|---|---|
| Deploy / DeployFromGit fails with `no online node with Docker or containerd` | Agents online but no runtime socket | Install Docker on a worker; restart agent; Nodes → Docker = yes |
| Opaque “internal error” on deploy | Older CP hid scheduler reasons | Upgrade control plane; check cordon/drain / runtime |
| Agent offline | Process dead, wrong URL, firewall | `norix-agent check`; agent logs; allow worker → CP |
| UI version badge missing (local Vite) | `/status` not proxied | Vite must proxy `/status` |
| Agents on different versions | Partial upgrades | Nodes → **Update agent**, or `norix-agent update --version …` |
| Dashboard login loop | Wrong `NORIX_WEB_ORIGIN` / cookies / HTTPS mismatch | Align public URL and CORS with how you open the UI |

## Installer failures

**Docker mode (`get-norix.sh`)**

- Docker / Compose missing → installer tries `get.docker.com` unless `NORIX_SKIP_DOCKER=1`.
- Port busy → set `NORIX_HTTP_PORT` or free the port.
- Clone / DNS errors → set `NORIX_REPO` / `NORIX_REF`, or copy source to the install dir.
- CP exits immediately after upgrade → ensure `NORIX_BUILD_SECRETS_KEY` is set (64 hex chars) or (local only) `NORIX_ALLOW_INSECURE_DEV_SECRETS=1`.

**Bare mode**

- Preflight fails on memory / openssl / curl → fix host packages first ([`BARE_INSTALL.md`](../runbooks/BARE_INSTALL.md)).
- SHA256 mismatch → download matching `SHA256SUMS` for the tag; avoid `--skip-verify` in production.
- systemd offline (containers/labs) → `--supervise none`.

## Agent enrollment

1. Mint a join token in **Regions & nodes**.
2. Run `install-agent.sh` with `--control-plane` and `--token`.
3. If you see a **WARNING** about missing Docker/containerd sockets, enroll still works but this node will not schedule builds/deploys until a runtime exists ([Nodes](./nodes-agents.md)).
4. Confirm **online** in the UI.

```bash
NORIX_CONTROL_PLANE_URL=https://YOUR_CP norix-agent check
# Logs: journalctl -u norix-agent -f   or   tail -f /var/lib/norix-agent/norix-agent.log
```

Common failures: wrong CP URL, clock skew, firewall blocking worker → CP, expired/used join token.

## Builds failing

| Signal | Check |
|---|---|
| Build never starts | ≥1 online node with Docker/containerd; node not cordoned |
| Build starts then errors | Dockerfile path; Git URL reachable from the **worker**; build logs in UI |
| Git private repo | Credentials / network from the worker |
| Secrets errors | `NORIX_BUILD_SECRETS_KEY` must be set on the CP for encrypted build secrets |

## Webhooks not firing

Deploy webhook: `POST /hooks/github/{token}`

1. Mint token via `POST /api/webhooks/workloads/{id}` (session Bearer).
2. GitHub → Webhooks: Payload URL = `https://YOUR_CP/hooks/github/<token>`.
3. **Secret** = the same `<token>` (HMAC `X-Hub-Signature-256` required).
4. Content type `application/json`; event `push`.
5. Delivery → 401 means bad/missing signature; 404 workload; 400 build/create errors.

Preview webhook (`/api/webhooks/github`) requires `NORIX_GITHUB_WEBHOOK_SECRET` (503 if unset).

See [Deploying → GitHub push webhook](./deploying.md).

## DNS / TLS issues

- Panel opens on HTTP but cookies fail on HTTPS → set `NORIX_PUBLIC_URL` / `NORIX_WEB_ORIGIN` to the HTTPS URL.
- Certificate errors on panel → terminate TLS on Caddy/nginx in front of `HTTP_ADDR` ([Production TLS](./production-tls.md)).
- App hostname no cert → DNS must point at edge; ACME / proxy must be configured; wait for propagation.
- Lab Pebble CA → browsers will not trust it; use a real CA in production.

## KVM / micro-VM missing

Micro-VM isolation is **Experimental** (`NORIX_MICROVM_*`). If a workload requires KVM and no node reports it:

- Error resembles `unschedulable: microvm_required but no node with KVM`.
- Ensure `/dev/kvm` exists, agent capabilities show `kvm_level ≥ 1`, and you are not in a nested VM without nested virt.
- For normal containers, leave micro-VM off and use Docker/containerd workers.

## CLI checks

```bash
norix-control-plane check
curl -sf http://127.0.0.1:8080/healthz
curl -sf http://127.0.0.1:8080/status | jq .

NORIX_CONTROL_PLANE_URL=https://YOUR_CP norix-agent check
```

## Get help

- GitHub Issues: https://github.com/xiron902224/norix/issues  
- Security: [`SECURITY.md`](../../SECURITY.md) (private reports only)
- Status / maturity: [`ESTADO_Y_PLAN_V1.md`](../ESTADO_Y_PLAN_V1.md)
