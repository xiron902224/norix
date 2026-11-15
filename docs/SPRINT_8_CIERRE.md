# Sprint 8 — Red entre nodos

## Objetivo (plan S0–S22)

Malla WireGuard, DNS interno, aislamiento por proyecto (nftables), egress por workload, certificados compartidos / wildcard.

## Estado

**Cerrado a nivel de plano de control + artifacts (2026-11-13).**  
Apply en vivo (`wg-quick` / `nft`) queda **opt-in** vía `NORIX_APPLY_WIREGUARD=1` (requiere capabilities en el host).

## Entregables

- Overlay `10.88.0.0/16`, FQDN `*.norix.internal`, keys WireGuard, `RenderWGQuick`.
- DNS hosts snapshot (`RenderHostsFile`) + plan en `GetDesiredNetworkPlan`.
- nftables: marcas por proyecto + drop cross-project + egress allow/deny/allowlist.
- Wildcard self-signed para edge multi-nodo (`IssueSelfSignedWildcard`).
- Local runtime + agente escriben `wg-norix.conf`, `norix.nft`, `dns-hosts`, certs; apply real si `plan.Apply`.
- Tests en `internal/network`.

## DoD

| Criterio | Estado |
|---|---|
| Config mesh WG generada | Cumple |
| DNS interno (hosts) por servicio | Cumple (unit) |
| Aislamiento proyecto en nftables | Cumple (unit) |
| Wildcard cert | Cumple (unit) |
| Apply vivo en 2 nodos | Opt-in / manual (`NORIX_APPLY_WIREGUARD=1`) |

## Cómo verificar

```bash
cd control-plane && go test ./internal/network/ ./internal/scheduler/ -count=1
```

Manual: 2 nodos, `NORIX_APPLY_WIREGUARD=1`, Network UI → generar plan → comprobar `wg show` y resolución `*.norix.internal` vía hosts.
