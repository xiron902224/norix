#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${NORIX_ENV_FILE:-$ROOT/control-plane/.env}"
FILE="${1:-}"
[[ -n "$FILE" && -f "$FILE" ]] || { echo "usage: $0 <backup.sql>"; exit 1; }
# shellcheck disable=SC1090
set -a && source "$ENV_FILE" && set +a
DSN="${MIGRATION_DATABASE_URL:-$DATABASE_URL}"
psql "$DSN" -v ON_ERROR_STOP=1 -f "$FILE"
echo "restored: $FILE"
