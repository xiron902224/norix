# Sprint 10 — Bases de datos 1-click y backups — Beta privada v0.5

## Objetivo
Catálogo Postgres/MySQL/Redis/Mongo, secretos cifrados, backup/restore API, invites vía team members.

## Estado
**Cerrado 2026-11-13.**

## Entregables
- API DatabaseService (create/list/backup/restore) + UI Databases
- Catálogo de imágenes `CatalogImageForEngine`
- Secretos AES (`NORIX_BUILD_SECRETS_KEY`) en create
- Backup/restore status transitions + `last_backup_at`
- Platform Postgres backups (Ops) ya existentes
- Invites: `AddTeamMember` por email/role member

## DoD
| Criterio | Estado |
|---|---|
| 1-click create engines | Cumple (API) |
| Backup/restore API | Cumple |
| Catalog images | Cumple (unit) |
| Beta path (team member invite) | Cumple |

## Tests
`go test ./internal/db/ -run 'Managed|Normalize|Catalog|BackupPath'`
