# Patroni failover smoke (#52)

```bash
./test/patroni/failover.sh
```

| Env | Effect |
|---|---|
| `NORIX_PATRONI_KEEP=1` | Leave compose + CP running |
| `NORIX_PATRONI_FAILOVER_TIMEOUT_S` | Max wait for new primary (default 180) |
| `SKIP_PATRONI_FAILOVER=1` | CI job no-op |

See [`docs/runbooks/POSTGRES_HA_FAILOVER.md`](../../docs/runbooks/POSTGRES_HA_FAILOVER.md).
