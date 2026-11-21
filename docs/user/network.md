# Network & edge

## WireGuard mesh (#48)

Overlay `10.88.0.0/16` with FQDN `*.norix.internal`. The control plane includes a DesiredNetworkPlan on heartbeats.

| Mode | How |
|---|---|
| **Apply on (default)** | `NORIX_APPLY_WIREGUARD` unset / `1` / `true` — agents run `wg-quick` + `nft` when those tools exist |
| **Dry-run** | `NORIX_APPLY_WIREGUARD=0` (or `false` / `off` / `dry-run`) — write `wg-norix.conf` / `norix.nft` / DNS snapshot only |
| **Unsupported host** | If `wg-quick` or `nft` is missing, apply is skipped safely (configs still written) |

On apply failure the agent **rolls back** to the previous WireGuard (and nft) config when a prior copy exists.

### Key rotation

`AgentService.RotateNodeWireGuardKeys` (owner/admin session) replaces one node's keypair, keeps the overlay address, audits `node.wg_rotate`, and bumps `network_generation` for the whole org so peers refresh.

### Connectivity contract

Workload DNS (`ServiceFQDN`) points at peer overlay addresses that appear in WireGuard `AllowedIPs`. Unit test: `go test ./internal/network/ -run Connectivity -count=1`. Lab smoke: `./test/mesh/connectivity.sh`.

## Networks page

Dashboard **Network** shows overlay status (`GetNetworkStatus`), including `apply_enabled`.

## Edge

Edge routing (Caddy) terminates TLS and forwards to workload upstreams. Agents may apply edge plans when acting as edge nodes.

If HTTPS links look wrong, check `NORIX_EDGE_HTTPS_PORT` / UI env (`VITE_EDGE_HTTPS_PORT` in local Vite) and your reverse proxy mapping.
