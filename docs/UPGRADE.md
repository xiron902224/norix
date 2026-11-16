# Upgrade guide (N-1 → N)

Full operator runbook (UI remote agent update, uninstall): [`UPDATE_AND_UNINSTALL.md`](./UPDATE_AND_UNINSTALL.md).

1. Backup Postgres (`Ops` UI or `scripts/backup-postgres.sh`).
2. Update control plane (`get-norix.sh` / `install.sh upgrade`).
3. Update agents from **Nodes → Update agent** or `install-agent.sh --update --version vX.Y.Z` / `norix-agent update`.
4. Verify `/healthz`, `/status`, one deploy, and Domains HTTPS.

Rollback: restore DB backup, redeploy previous binary, restart agents on previous version.

## CI smoke

GitHub Actions job **Upgrade smoke (N-1 → N)** runs `scripts/ci-upgrade-smoke.sh`:

1. Download previous release control-plane binary (`NORIX_UPGRADE_FROM`, default `v1.0.0-beta.3`)
2. Migrate empty DB as N-1
3. Seed a marker workload
4. Build current CP and migrate to N
5. Assert `schema_migrations` is clean and the marker workload survives

Bump `NORIX_UPGRADE_FROM` when cutting a new beta so CI always tests the previous published tag.
