# Production TLS

One path from domain → DNS → certificates → panel HTTPS → app domains, for **Docker** and **bare** installs.

Norix control plane listens on plain HTTP by default (`HTTP_ADDR`). Put TLS in front (Caddy/nginx) or enable ACME DNS-01 for **app** hostnames via the edge. Do not expose an unauthenticated admin UI on the public internet over plain HTTP.

**First-admin bootstrap (#6):** `CreateInitialAdmin` is allowed only from loopback, over HTTPS (with `X-Forwarded-Proto: https` from a loopback proxy), or when `NORIX_ALLOW_INSECURE_BOOTSTRAP=1`. Prefer finishing TLS (this guide) or an SSH tunnel before creating the owner account.

## 1. Choose names

| Name | Example | Purpose |
|---|---|---|
| Panel / API | `norix.example.com` | Dashboard + Connect API |
| Apps | `*.apps.example.com` or per-app hosts | Workload domains |

Set these in env (installer writes them):

```bash
NORIX_PUBLIC_URL=https://norix.example.com
NORIX_WEB_ORIGIN=https://norix.example.com
CORS_ALLOWED_ORIGINS=https://norix.example.com
```

Wildcard CORS (`*`) needs `NORIX_CORS_ALLOW_WILDCARD=1` — avoid in production.

## 2. DNS

| Record | Target |
|---|---|
| `norix.example.com` A/AAAA | Control-plane host (or load balancer) |
| App hostnames A/AAAA or CNAME | Edge host (often the same VPS, or dedicated edge nodes) |

Wait for propagation before requesting certificates.

## 3. Certificates for the panel

### Option A — reverse proxy (recommended)

Bind the control plane locally and terminate TLS on Caddy/nginx.

```bash
# control-plane.env / Compose
HTTP_ADDR=127.0.0.1:8080   # bare often uses :8000
NORIX_PUBLIC_URL=https://norix.example.com
NORIX_WEB_ORIGIN=https://norix.example.com
```

Minimal Caddy (Docker or host):

```caddy
norix.example.com {
  reverse_proxy 127.0.0.1:8080
}
```

Issue Let’s Encrypt via the proxy’s ACME (HTTP-01 on :80 or DNS-01).

### Option B — Docker Compose edge profile

Lab Compose may use **Pebble** (fake CA). For production, remove Pebble and use a real CA / external proxy. See [`BETA1_3NODE_INSTALL.md`](../runbooks/BETA1_3NODE_INSTALL.md).

### Docker mode checklist

1. Install with `get-norix.sh` (default Docker).
2. Point DNS at the host.
3. Put Caddy/nginx (or a cloud LB) in front of `NORIX_HTTP_PORT` (default `8000`) **or** set `HTTP_ADDR` to localhost and proxy only.
4. Open `https://norix.example.com` — cookie / CORS origins must match HTTPS.

### Bare mode checklist

1. Install `--mode bare` ([`BARE_INSTALL.md`](../runbooks/BARE_INSTALL.md)).
2. Prefer `HTTP_ADDR=127.0.0.1:8000`.
3. System Caddy/nginx → that port with a real certificate.
4. Align `NORIX_PUBLIC_URL` / `NORIX_WEB_ORIGIN` with the HTTPS URL.

## 4. Certificates for app domains

1. Create the domain in the dashboard ([Domains](./domains.md)) targeting a workload.
2. DNS for that hostname → edge / CP host.
3. When Caddy edge + ACME are configured, certificates are requested for assigned hostnames.

DNS-01 (control plane) example:

```bash
NORIX_ACME_DNS_PROVIDER=cloudflare
NORIX_ACME_EMAIL=ops@example.com
# plus provider token env as documented in the 3-node runbook
```

Without ACME, terminate TLS on your own reverse proxy to the published container port / edge upstream.

## 5. Verify

```bash
curl -sfI https://norix.example.com/healthz
curl -sf https://norix.example.com/status | jq .
# Browser: login on https://… (not http://) with matching NORIX_WEB_ORIGIN
```

App hostname should show a valid certificate and route to the workload.

## Related

- [Domains & TLS](./domains.md)
- [Troubleshooting](./troubleshooting.md)
- [Bare install](../runbooks/BARE_INSTALL.md)
- [3-node lab / prod notes](../runbooks/BETA1_3NODE_INSTALL.md)
