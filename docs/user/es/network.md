# Red y edge

## Mesh WireGuard (#48)

Overlay `10.88.0.0/16` y FQDN `*.norix.internal`. El control plane envía `DesiredNetworkPlan` en cada heartbeat.

| Modo | Cómo |
|---|---|
| **Apply activo (por defecto)** | `NORIX_APPLY_WIREGUARD` vacío / `1` — el agent ejecuta `wg-quick` + `nft` si existen |
| **Dry-run** | `NORIX_APPLY_WIREGUARD=0` (`false` / `off` / `dry-run`) — solo escribe configs |
| **Host sin herramientas** | Si faltan `wg-quick` o `nft`, se omite el apply (seguro) |

Si el apply falla, el agent **revierte** a la conf WireGuard/nft anterior cuando existe.

### Rotación de claves

RPC `RotateNodeWireGuardKeys`: nueva keypair, misma dirección overlay, audit `node.wg_rotate`, bump de `network_generation` en la org.

### Conectividad

DNS de workload → IP overlay del peer en `AllowedIPs`. Tests: `go test ./internal/network/ -run Connectivity -count=1` · `./test/mesh/connectivity.sh`.

Guía EN: [network.md](../network.md).
