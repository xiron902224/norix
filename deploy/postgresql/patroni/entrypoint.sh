#!/bin/sh
set -eu
DATA_DIR="${PATRONI_POSTGRESQL_DATA_DIR:-/home/postgres/pgdata/pgroot/data}"
mkdir -p "$DATA_DIR"
# Postgres refuses to start unless the data dir mode is 0700 or 0750 (#52 failover).
chmod 700 /home/postgres/pgdata 2>/dev/null || true
chmod 700 /home/postgres/pgdata/pgroot 2>/dev/null || true
chmod 700 "$DATA_DIR" 2>/dev/null || true
chown -R postgres:postgres /home/postgres/pgdata
sed \
  -e "s|__NAME__|${PATRONI_NAME}|g" \
  -e "s|__SCOPE__|${PATRONI_SCOPE}|g" \
  -e "s|__RESTAPI_CONNECT__|${PATRONI_RESTAPI_CONNECT_ADDRESS}|g" \
  -e "s|__PG_CONNECT__|${PATRONI_POSTGRESQL_CONNECT_ADDRESS}|g" \
  -e "s|__DATA_DIR__|${DATA_DIR}|g" \
  -e "s|__SUPERUSER_PASSWORD__|${PATRONI_SUPERUSER_PASSWORD}|g" \
  -e "s|__REPLICATION_PASSWORD__|${PATRONI_REPLICATION_PASSWORD}|g" \
  -e "s|__ETCD_HOSTS__|${PATRONI_ETCD3_HOSTS}|g" \
  /patroni/patroni-template.yml > /tmp/patroni.yml
chown postgres:postgres /tmp/patroni.yml
exec su-exec postgres patroni /tmp/patroni.yml
