#!/bin/sh
set -eu

apk add --no-cache patroni python3 py3-pip py3-psycopg2 py3-yaml curl >/dev/null

# Render a minimal patroni.yml from env (template uses simple sed placeholders).
sed \
  -e "s|__NAME__|${PATRONI_NAME}|g" \
  -e "s|__SCOPE__|${PATRONI_SCOPE}|g" \
  -e "s|__RESTAPI_CONNECT__|${PATRONI_RESTAPI_CONNECT_ADDRESS}|g" \
  -e "s|__PG_CONNECT__|${PATRONI_POSTGRESQL_CONNECT_ADDRESS}|g" \
  -e "s|__DATA_DIR__|${PATRONI_POSTGRESQL_DATA_DIR}|g" \
  -e "s|__SUPERUSER_PASSWORD__|${PATRONI_SUPERUSER_PASSWORD}|g" \
  -e "s|__REPLICATION_PASSWORD__|${PATRONI_REPLICATION_PASSWORD}|g" \
  -e "s|__ETCD_HOSTS__|${PATRONI_ETCD3_HOSTS}|g" \
  /patroni/patroni-template.yml > /tmp/patroni.yml

exec patroni /tmp/patroni.yml
