#!/bin/bash
set -euo pipefail

psql -v ON_ERROR_STOP=1 --username "${POSTGRES_USER}" --dbname "${POSTGRES_DB}" <<SQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'norix_app') THEN
    CREATE ROLE norix_app LOGIN PASSWORD '${POSTGRES_APP_PASSWORD}' NOINHERIT;
  ELSE
    ALTER ROLE norix_app WITH LOGIN PASSWORD '${POSTGRES_APP_PASSWORD}' NOINHERIT;
  END IF;
END
\$\$;
SQL
