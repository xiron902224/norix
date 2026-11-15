#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${NORIX_ENV_FILE:-$ROOT/control-plane/.env}"
# shellcheck disable=SC1090
set -a && source "$ENV_FILE" && set +a
DIR="${NORIX_BACKUP_DIR:-$HOME/.norix/backups}"
mkdir -p "$DIR"
STAMP="$(date -u +%Y%m%d-%H%M%S)"
OUT="$DIR/norix-$STAMP.sql"
DSN="${MIGRATION_DATABASE_URL:-$DATABASE_URL}"
pg_dump --no-owner --no-acl --dbname="$DSN" --file="$OUT"
sha256sum "$OUT" | tee "$OUT.sha256"
echo "backup: $OUT"
