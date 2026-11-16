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
