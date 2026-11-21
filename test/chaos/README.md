# Chaos scenarios (#41)

See **[docs/user/chaos.md](../../docs/user/chaos.md)** for expected behavior.

```bash
./test/chaos/run-all.sh          # setup → all scenarios → teardown
./test/chaos/agent-crash.sh --setup
NORIX_CHAOS_KEEP=1 ./test/chaos/control-plane-restart.sh --setup
```

Reuses `test/multinode/docker-compose.yml` (postgres + CP + agents).
