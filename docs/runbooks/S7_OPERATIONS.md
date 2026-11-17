# Runbook S7 — backups, alerts, upgrades

## Backup

```bash
./scripts/backup-postgres.sh
# or from UI: Ops → Create backup
```

Files land in `NORIX_BACKUP_DIR` (default `~/.norix/backups` via script, or temp dir from API).

## Restore

```bash
./scripts/restore-postgres.sh ~/.norix/backups/norix-YYYYMMDD-HHMMSS.sql
```

UI restore applies the SQL dump with `psql -v ON_ERROR_STOP=1`. Prefer a staging DB first.

## Rotate secrets

1. Change Postgres passwords in Compose `.env` and rotate `norix_app`.
2. Rotate `NORIX_BUILD_SECRETS_KEY` carefully — it encrypts **build secrets**, **TOTP**, **WireGuard**, **managed DB connection secrets**, and **workload env vars** (#8):
   - Schedule a maintenance window.
   - Keep the old key available until you have re-saved (or re-entered) all encrypted material under the new key.
   - Practical path: set the new key → re-enter workload env values and build secrets in the UI → verify deploys → discard the old key.
   - Losing the key makes stored ciphertext unreadable (by design; see ADR-003).
3. Recreate agent join tokens after node credential compromise.
4. Restart control-plane and agents.

## Upgrade

```bash
./scripts/install.sh upgrade
# rebuild binaries
make build && make agent
# restart control-plane / agent processes
```

## Alerts

Set `NORIX_ALERT_WEBHOOK_URL` to receive JSON posts for:

- `node.down` — remote agent heartbeat older than 45s
- `deploy.failed` — workload status became `failed`

UI: **Ops → Alerts**.
