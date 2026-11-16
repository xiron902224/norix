#!/usr/bin/env bash
# N-1 → N migration smoke for CI.
# Downloads the previous control-plane release, migrates, seeds a marker workload,
# then migrates with the current tree binary and asserts data + schema survive.
set -euo pipefail

PREV_TAG="${NORIX_UPGRADE_FROM:-v1.0.0-beta.3}"
REPO="${NORIX_GITHUB_REPO:-xiron902224/norix}"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

DATABASE_URL="${DATABASE_URL:?DATABASE_URL required}"
MIGRATION_DATABASE_URL="${MIGRATION_DATABASE_URL:-$DATABASE_URL}"
ORG_ID='11111111-1111-1111-1111-111111111111'
PROJECT_ID='33333333-3333-3333-3333-333333333333'
WORKLOAD_ID='44444444-4444-4444-4444-444444444444'

echo "==> N-1 binary ${PREV_TAG} (${ARCH})"
curl -fsSL -o "$WORKDIR/cp-n1" \
  "https://github.com/${REPO}/releases/download/${PREV_TAG}/norix-control-plane-linux-${ARCH}"
chmod +x "$WORKDIR/cp-n1"

echo "==> migrate with N-1"
DATABASE_URL="$DATABASE_URL" MIGRATION_DATABASE_URL="$MIGRATION_DATABASE_URL" \
  "$WORKDIR/cp-n1" -migrate-only

N1_VERSION="$(psql "$MIGRATION_DATABASE_URL" -Atc 'SELECT version FROM schema_migrations')"
echo "schema after N-1: version=${N1_VERSION}"

echo "==> seed marker tenant + workload"
psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 <<SQL
SELECT set_config('app.current_tenant_id', '${ORG_ID}', false);

INSERT INTO organizations (id, name, slug)
VALUES ('${ORG_ID}', 'Upgrade Smoke Org', 'upgrade-smoke')
ON CONFLICT (id) DO NOTHING;

INSERT INTO projects (id, organization_id, name)
VALUES ('${PROJECT_ID}', '${ORG_ID}', 'upgrade-smoke')
ON CONFLICT (id) DO NOTHING;

INSERT INTO workloads (
  id, organization_id, project_id, name, runtime, image_or_source, status, container_port
) VALUES (
  '${WORKLOAD_ID}',
  '${ORG_ID}',
  '${PROJECT_ID}',
  'marker',
  'container',
  'nginx:alpine',
  'stopped',
  80
) ON CONFLICT (id) DO NOTHING;
SQL

echo "==> build current control-plane"
(cd "$ROOT/control-plane" && CGO_ENABLED=0 go build -o "$WORKDIR/cp-n" ./cmd/server)

echo "==> migrate with N (current)"
DATABASE_URL="$DATABASE_URL" MIGRATION_DATABASE_URL="$MIGRATION_DATABASE_URL" \
  "$WORKDIR/cp-n" -migrate-only

N_VERSION="$(psql "$MIGRATION_DATABASE_URL" -Atc 'SELECT version FROM schema_migrations')"
DIRTY="$(psql "$MIGRATION_DATABASE_URL" -Atc 'SELECT dirty FROM schema_migrations')"
echo "schema after N: version=${N_VERSION} dirty=${DIRTY}"

if [[ "$DIRTY" != "f" && "$DIRTY" != "false" ]]; then
  echo "schema_migrations.dirty must be false" >&2
  exit 1
fi
if [[ "$N_VERSION" -lt "$N1_VERSION" ]]; then
  echo "schema version went backwards: ${N1_VERSION} → ${N_VERSION}" >&2
  exit 1
fi

SURVIVED="$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -At <<SQL
SELECT set_config('app.current_tenant_id', '${ORG_ID}', false);
SELECT count(*) FROM workloads WHERE id = '${WORKLOAD_ID}' AND image_or_source = 'nginx:alpine';
SQL
)"
SURVIVED="$(echo "$SURVIVED" | tail -n1 | tr -d '[:space:]')"

if [[ "$SURVIVED" != "1" ]]; then
  echo "marker workload did not survive upgrade (count=${SURVIVED})" >&2
  exit 1
fi

echo "OK: upgraded ${PREV_TAG} (v${N1_VERSION}) → current (v${N_VERSION}); workload survived"
