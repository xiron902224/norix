# Domains & TLS

## Assign a domain

1. Open **Domains** (or assign from a workload)
2. Choose hostname and target workload / upstream
3. Ensure DNS points at your edge / control-plane host (A/AAAA or CNAME as designed for your install)

## TLS

When Caddy / ACME is configured on the install, certificates are requested automatically for assigned hostnames. Local labs may use Pebble or HTTP-only — check your operator’s install notes ([`BARE_INSTALL`](../runbooks/BARE_INSTALL.md), compose edge profile).

## Tips

- Use the public URL printed at install (`NORIX_PUBLIC_URL`) as the browser origin for the dashboard when behind TLS
- After changing routes, give agents/edge a short moment to apply the desired edge plan
