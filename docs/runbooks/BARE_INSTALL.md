# Bare / VPS install (sin Docker)

Instala el **control plane** de Norix como binario + systemd, con PostgreSQL del sistema (o uno externo). **No requiere Docker** para levantar el panel.

> Container workloads (`nginx:alpine`, git builds, etc.) sí necesitan Docker (u otro runtime) en **algún** nodo — este u otro VPS con el agent. Bare solo evita Docker para el propio Norix.

## Requisitos

- Linux amd64 o arm64, root/sudo  
- Puerto libre (default `8000`)  
- Salida HTTPS (descarga de release / git)  
- PostgreSQL 16+ (el installer puede instalarlo vía `apt`/`dnf`)

## One-liner

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/v1.0.0-beta.2/scripts/get-norix.sh \
  | NORIX_REF=v1.0.0-beta.2 bash -s -- --mode bare
```

O desde `develop`:

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/develop/scripts/get-norix.sh \
  | bash -s -- --mode bare
```

### Qué hace

1. Clona el repo en `/data/norix/source`  
2. Instala PostgreSQL si no está  
3. Crea DB `norix` + roles `norix` / `norix_app`  
4. Descarga `norix-control-plane-linux-{amd64,arm64}` del release (o `make build` si no hay asset)  
5. Escribe `/etc/norix/control-plane.env`  
6. Activa `norix-control-plane.service`  
7. Espera `http://127.0.0.1:8000/healthz`

Abre la URL impresa → bootstrap del primer admin.

## Proceso manual (sin el script)

### 1. PostgreSQL

```bash
# Debian/Ubuntu
sudo apt-get update && sudo apt-get install -y postgresql postgresql-contrib
sudo systemctl enable --now postgresql

sudo -u postgres createuser -s norix   # o CREATE ROLE …
sudo -u postgres createdb -O norix norix
# Crear norix_app con password y grants (ver scripts/get-norix.sh ensure_norix_db_bare)
```

DB gestionada (RDS, Neon, etc.): solo necesitas dos URLs con SSL según el proveedor.

### 2. Binario del control plane

```bash
TAG=v1.0.0-beta.2
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -fsSL -O "https://github.com/xiron902224/norix/releases/download/${TAG}/norix-control-plane-linux-${ARCH}"
curl -fsSL -O "https://github.com/xiron902224/norix/releases/download/${TAG}/SHA256SUMS"
sha256sum -c SHA256SUMS --ignore-missing
sudo install -m 755 "norix-control-plane-linux-${ARCH}" /usr/local/bin/norix-control-plane
```

Desde source (si no hay release):

```bash
git clone https://github.com/xiron902224/norix.git && cd norix
git checkout "$TAG"
make build
sudo install -m 755 control-plane/bin/norix-control-plane /usr/local/bin/norix-control-plane
```

### 3. Env + usuario

```bash
sudo useradd --system --home /var/lib/norix --shell /usr/sbin/nologin norix || true
sudo mkdir -p /etc/norix /var/lib/norix/backups
sudo tee /etc/norix/control-plane.env >/dev/null <<'EOF'
DATABASE_URL=postgres://norix_app:CHANGE_APP@127.0.0.1:5432/norix?sslmode=disable
MIGRATION_DATABASE_URL=postgres://norix:CHANGE_OWNER@127.0.0.1:5432/norix?sslmode=disable
HTTP_ADDR=0.0.0.0:8000
LOG_LEVEL=info
CORS_ALLOWED_ORIGINS=*
NORIX_PUBLIC_URL=http://YOUR_IP:8000
NORIX_WEB_ORIGIN=http://YOUR_IP:8000
NORIX_BACKUP_DIR=/var/lib/norix/backups
NORIX_BUILD_SECRETS_KEY=   # openssl rand -hex 32
NORIX_REGISTRY=none
EOF
sudo chmod 600 /etc/norix/control-plane.env
sudo chown norix:norix /etc/norix/control-plane.env /var/lib/norix
```

### 4. systemd

```bash
sudo cp deploy/systemd/norix-control-plane.service /etc/systemd/system/
# Opcional: After=postgresql.service (sin depender de docker.service)
sudo systemctl daemon-reload
sudo systemctl enable --now norix-control-plane
curl -sf http://127.0.0.1:8000/healthz
```

## Después del panel

| Objetivo | Cómo |
|---|---|
| Solo UI / API / auth | Ya listo en bare |
| Desplegar contenedores en **este** VPS | Instalar Docker + agent (`install-agent.sh`) o usar modo `docker` del installer |
| Desplegar en **otros** VPS | Join tokens + agent en cada worker ([BETA1_3NODE_INSTALL.md](./BETA1_3NODE_INSTALL.md)) |
| TLS público | Reverse proxy (Caddy/nginx) → `127.0.0.1:8000` o `HTTP_ADDR=127.0.0.1:8000` |

## Upgrade bare

```bash
# Nuevo binario del tag
sudo systemctl stop norix-control-plane
# install -m 755 norix-control-plane-linux-… /usr/local/bin/norix-control-plane
sudo systemctl start norix-control-plane
```

O re-ejecutar el one-liner con `--mode bare` (conserva `/etc/norix/control-plane.env`).

## Diff vs modo Docker

| | `--mode bare` | `--mode docker` (default) |
|---|---|---|
| Postgres | Paquete del OS / externo | Contenedor Compose |
| Control plane | Binario + systemd | Contenedor + `docker.sock` |
| Primer worker embebido | No (hasta instalar Docker/agent) | Sí (`localruntime` vía sock) |
| Requiere Docker | No | Sí |
