# Ops & backups

**Deployments / Ops** covers operational actions for the platform:

- list / create **backups**
- download / restore (when enabled)
- alert events
- related audit / usage exports (API)

## Recommended habit

1. Backup before upgrades ([`UPDATE_AND_UNINSTALL.md`](../UPDATE_AND_UNINSTALL.md))
2. Verify `/healthz` and `/status` after changes
3. Confirm agents online and versions match

Operators can also use `scripts/backup-postgres.sh` / host `pg_dump` for the control-plane database.

## Log retention

The control-plane leader prunes `workload_logs`, `build_logs`, and `log_segments` about every 20 seconds. Each table is cut by age, by number of lines, and by total bytes. The workload page states these defaults above the log panels. An image deploy has runtime logs only; build logs appear after a git build. Defaults:

| Variable | Default | Meaning |
|---|---|---|
| `NORIX_LOG_RETENTION_DAYS` | `14` | Drop rows older than this. `0` disables age prune. |
| `NORIX_WORKLOAD_LOG_MAX_LINES` | `500` | Newest lines kept per workload. |
| `NORIX_BUILD_LOG_MAX_LINES` | `800` | Newest lines kept per build. |
| `NORIX_LOG_SEGMENT_MAX_LINES` | `5000` | Newest segment rows kept per organization. |
| `NORIX_LOG_RETENTION_BYTES` | `33554432` (32 MiB) | Newest bytes kept per workload. `0` disables the byte cap. |
| `NORIX_BUILD_LOG_RETENTION_BYTES` | `33554432` | Newest bytes kept per build. |
| `NORIX_LOG_SEGMENT_RETENTION_BYTES` | `67108864` (64 MiB) | Newest bytes kept per organization. |

A non-numeric value falls back to the default. Line caps clamp into their allowed range. Byte values outside 0–10 GiB fall back to the default. The line cap also applies when the agent reports logs, so a burst cannot wait for the next prune. Restart the control plane after changing these variables.

## Restore

Restore replaces the `public` schema of the control-plane database with the dump. A plain `pg_dump` only contains `CREATE` statements, so loading it into a database that already has `alert_events` fails with `relation already exists` (psql exit 3).

The control plane now dumps with `--clean --if-exists` and, before `psql -f`, disconnects other sessions and runs `DROP SCHEMA public CASCADE`. Restart the control plane after a restore so its pool reconnects. Old dump files (without `DROP`) restore the same way.
