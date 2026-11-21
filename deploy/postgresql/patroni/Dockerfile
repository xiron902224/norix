# Local Patroni+Postgres image for Norix HA lab (#52).
FROM postgres:16-alpine
USER root
RUN apk add --no-cache \
      python3 py3-pip py3-psycopg2 py3-yaml curl su-exec \
      gcc musl-dev python3-dev libffi-dev \
    && pip3 install --no-cache-dir --break-system-packages 'patroni[etcd3]==4.0.4' \
    && apk del gcc musl-dev python3-dev libffi-dev \
    && mkdir -p /home/postgres/pgdata \
    && chown -R postgres:postgres /home/postgres
