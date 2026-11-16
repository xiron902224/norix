# Upgrade guide (N-1 → N)

Full operator runbook (UI remote agent update, uninstall): [`UPDATE_AND_UNINSTALL.md`](./UPDATE_AND_UNINSTALL.md).

1. Backup Postgres (`Ops` UI or `scripts/backup-postgres.sh`).
2. Update control plane (`get-norix.sh` / `install.sh upgrade`).
3. Update agents from **Nodes → Update agent** or `install-agent.sh --update --version vX.Y.Z` / `norix-agent update`.
4. Verify `/healthz`, `/status`, one deploy, and Domains HTTPS.

Rollback: restore DB backup, redeploy previous binary, restart agents on previous version.
