# Network & edge

## Networks page

Shows overlay / WireGuard-related status when enabled (`NORIX_NETWORK_APPLY` and related flags on the control plane). Multi-node labs can verify peers and DNS records here.

## Edge

Edge routing (Caddy) terminates TLS and forwards to workload upstreams. Agents may apply edge plans when acting as edge nodes.

If HTTPS links look wrong, check `NORIX_EDGE_HTTPS_PORT` / UI env (`VITE_EDGE_HTTPS_PORT` in local Vite) and your reverse proxy mapping.
