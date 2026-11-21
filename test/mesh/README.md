# Mesh connectivity (#48)

```bash
./test/mesh/connectivity.sh
```

Validates the workload-to-workload overlay contract (FQDN → overlay IP → peer `AllowedIPs`) and that WireGuard apply defaults on with a dry-run opt-out. Live `wg show` between two CAP_NET_ADMIN hosts remains a manual lab drill (see `docs/user/network.md`).
