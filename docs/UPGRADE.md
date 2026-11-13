# Upgrade guide (N-1 → N)

1. Backup Postgres (`Ops` UI or `scripts/backup-postgres.sh`).
2. Pull new release / `git checkout vX.Y.Z`.
3. Run control plane once (`go run` or binary) to apply migrations.
4. Upgrade agents (`install-agent.sh` / restart with same state dir).
5. Verify `/healthz`, one deploy, and Domains HTTPS.

Rollback: restore DB backup, redeploy previous binary, restart agents on previous version.
