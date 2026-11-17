#!/usr/bin/env bash
# Smoke for panel TLS overlays (#26): compose merge + installer flags + Caddyfile.
# Full ACME against Pebble is optional (NORIX_PANEL_TLS_E2E=1) — needs Docker pull.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail() { echo "error: $*" >&2; exit 1; }

[[ -f deploy/norix/docker-compose.panel-tls.yml ]] || fail "missing panel-tls compose"
[[ -f deploy/norix/Caddyfile.panel ]] || fail "missing Caddyfile.panel"
grep -q -- '--panel-domain' scripts/get-norix.sh || fail "installer missing --panel-domain"
grep -q 'docker-compose.panel-tls.yml' scripts/get-norix.sh || fail "installer not wiring panel-tls overlay"

TMP="$(mktemp)"
cat >"$TMP" <<EOF
POSTGRES_PASSWORD=x
POSTGRES_APP_PASSWORD=x
DATABASE_URL=postgres://norix_app:x@postgres:5432/norix?sslmode=disable
MIGRATION_DATABASE_URL=postgres://norix:x@postgres:5432/norix?sslmode=disable
NORIX_PANEL_DOMAIN=panel.norix.test
NORIX_ACME_EMAIL=test@norix.test
NORIX_HTTP_PORT=8000
NORIX_BUILD_SECRETS_KEY=$(openssl rand -hex 32)
EOF

docker compose --env-file "$TMP" \
  -f deploy/norix/docker-compose.prod.yml \
  -f deploy/norix/docker-compose.panel-tls.yml \
  config >/dev/null
ok_merge=1
rm -f "$TMP"
echo "OK compose merge panel-tls"

if [[ "${NORIX_PANEL_TLS_E2E:-0}" == "1" ]]; then
  echo "NORIX_PANEL_TLS_E2E=1 — starting pebble stack (long)…"
  # Delegated to operators; keep smoke fast by default.
  fail "full E2E not enabled in CI yet; set up DNS/Pebble volume manually"
fi

echo "OK panel TLS smoke"
