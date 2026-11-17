# Bare / VPS install (sin Docker)

Instala el **control plane** de Norix como binario + PostgreSQL del sistema. **Un solo comando** (estilo OpenShip): no hace falta crear DB, bajar el binario ni escribir el unit a mano.

> Container workloads (`nginx:alpine`, git builds, etc.) sí necesitan Docker (u otro runtime) en **algún** nodo — este u otro VPS con el agent. Bare solo evita Docker para el propio Norix.

## Requisitos (VPS real)

- Linux amd64 o arm64, **root**
- **systemd** como PID 1 (Ubuntu/Debian VPS normal)
- Puerto libre (default `8000`)
- Salida HTTPS (GitHub Releases + git)
- ~1 GiB RAM recomendada

## One-click

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/v1.0.0-beta.5/scripts/get-norix.sh \
  | NORIX_REF=v1.0.0-beta.5 bash -s -- --mode bare
```

Desde `main` (tip sin tag):

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/main/scripts/get-norix.sh \
  | bash -s -- --mode bare
```

Flags útiles:

```bash
bash -s -- --mode bare --port 8000 --public-url https://norix.example.com
```

### Qué hace (automático)

1. Preflight (arch, systemd, puerto, RAM cgroup)
2. Clona/sync del repo en `/data/norix/source`
3. Instala/arranca PostgreSQL
4. Crea DB `norix` + roles `norix` / `norix_app` (idempotente, sin depender de `sudo`)
5. Descarga el binario del release y verifica **SHA256SUMS** (+ **cosign** si el release trae `.cosign.bundle`; no compila)
6. Escribe `/etc/norix/control-plane.env` (se conserva en re-runs)
7. Activa `norix-control-plane.service` **o** nohup si `--supervise none`
8. Espera `http://127.0.0.1:8000/healthz` y escribe `/var/lib/norix/install-state.json`

Abre la URL impresa → bootstrap del primer admin.

> Bare **no** activa `NORIX_ALLOW_INSECURE_BOOTSTRAP` por defecto. Usa HTTPS ([production TLS](../user/production-tls.md)) o un túnel:
> `ssh -L 8000:127.0.0.1:8000 user@host` → `http://127.0.0.1:8000`.

Re-ejecutar el mismo comando es **seguro** (upgrade / resume).

## Labs / contenedores sin systemd

Si PID 1 no es systemd (p. ej. lab Docker `sleep infinity`):

```bash
curl -fsSL …/get-norix.sh \
  | NORIX_REF=v1.0.0-beta.5 bash -s -- --mode bare --supervise none
```

- Logs: `/var/lib/norix/norix-control-plane.log`
- PID: `/var/lib/norix/norix-control-plane.pid`
- Stop: `kill $(cat /var/lib/norix/norix-control-plane.pid)`

Sin `--supervise none`, el installer **aborta** con un mensaje claro (no deja Postgres a medias sin panel).

Offline / red lenta a GitHub Releases:

```bash
NORIX_BINARY_FILE=/path/to/norix-control-plane-linux-amd64 \
NORIX_SKIP_CHECKSUM=1 \
  bash get-norix.sh --mode bare --supervise none
```

(En VPS normal el one-liner descarga el asset, verifica SHA256SUMS y cosign si hay bundle; no uses `NORIX_SKIP_CHECKSUM` / `NORIX_SKIP_COSIGN` en producción.)

## Proceso manual (sin el script)

Solo si necesitas depurar. Preferir el one-liner.

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
TAG=v1.0.0-beta.5
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
ASSET="norix-control-plane-linux-${ARCH}"
curl -fsSL -O "https://github.com/xiron902224/norix/releases/download/${TAG}/${ASSET}"
curl -fsSL -O "https://github.com/xiron902224/norix/releases/download/${TAG}/SHA256SUMS"
curl -fsSL -O "https://github.com/xiron902224/norix/releases/download/${TAG}/${ASSET}.cosign.bundle" || true
sha256sum -c SHA256SUMS --ignore-missing
if [[ -f "${ASSET}.cosign.bundle" ]]; then
  cosign verify-blob \
    --bundle "${ASSET}.cosign.bundle" \
    --certificate-identity-regexp 'https://github.com/xiron902224/norix/\.github/workflows/release\.yml@.*' \
    --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
    "${ASSET}"
fi
sudo install -m 755 "${ASSET}" /usr/local/bin/norix-control-plane
```

Flujo “bajar → verificar → ejecutar” también en [`README.md`](../../README.md#verify-release-binaries-download--verify--run) / [`README.es.md`](../../README.es.md#verificar-binarios-del-release-bajar--verificar--ejecutar).

### 3. Env + usuario

```bash
sudo useradd --system --home /var/lib/norix --shell /usr/sbin/nologin norix || true
sudo mkdir -p /etc/norix /var/lib/norix/backups
sudo tee /etc/norix/control-plane.env >/dev/null <<'EOF'
DATABASE_URL=postgres://norix_app:CHANGE_APP@127.0.0.1:5432/norix?sslmode=disable
MIGRATION_DATABASE_URL=postgres://norix:CHANGE_OWNER@127.0.0.1:5432/norix?sslmode=disable
HTTP_ADDR=0.0.0.0:8000
LOG_LEVEL=info
CORS_ALLOWED_ORIGINS=http://YOUR_IP:8000
NORIX_PUBLIC_URL=http://YOUR_IP:8000
NORIX_WEB_ORIGIN=http://YOUR_IP:8000
# Wildcard CORS requires explicit opt-in: CORS_ALLOWED_ORIGINS=* and NORIX_CORS_ALLOW_WILDCARD=1
NORIX_BACKUP_DIR=/var/lib/norix/backups
NORIX_BUILD_SECRETS_KEY=   # openssl rand -hex 32
NORIX_REGISTRY=none
EOF
sudo chmod 600 /etc/norix/control-plane.env
sudo chown norix:norix /etc/norix/control-plane.env /var/lib/norix
```

### 4. systemd

```bash
sudo cp deploy/systemd/norix-control-plane.bare.service /etc/systemd/system/norix-control-plane.service
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
| Agent en lab sin systemd | `install-agent.sh` con `--supervise none` (o `auto`: nohup + `WORKDIR/norix-agent.log`) |
| Builds Git / contenedores | Docker o containerd en **workers** (el CP bare solo no basta) |
| TLS público | Reverse proxy (Caddy/nginx) → `127.0.0.1:8000` o `HTTP_ADDR=127.0.0.1:8000` |

### Un worker `online` no acepta builds Git

El control plane en modo bare puede funcionar sin Docker. Cada nodo que vaya a
ejecutar builds Git o contenedores necesita un runtime accesible al agent.
Un agent sin runtime puede seguir enviando heartbeats y aparecer como `online`,
pero el scheduler lo descarta con `no container runtime`.

1. Revisa el worker en la respuesta de `AgentService.ListNodes`. Los campos
   protobuf `capabilities.docker_socket` y `capabilities.containerd_socket` se
   llaman `dockerSocket` y `containerdSocket` en JSON. La tabla `nodes` almacena
   los mismos dos indicadores como `docker_socket` y `containerd_socket`.
2. Si ambos son falsos, instala y activa Docker o un stack de containerd con
   `nerdctl` en el worker. Comprueba que el usuario del servicio del agent puede
   ejecutar `docker info` o `nerdctl info`. Para builds con `nerdctl`, comprueba
   también que [BuildKit esté disponible](https://github.com/containerd/nerdctl/blob/main/docs/build.md).
3. Reinicia el agent para que vuelva a detectar el runtime. Confirma que el
   indicador correspondiente esté activo y reintenta el build. Un socket
   detectado por sí solo no garantiza que el comando de build funcione.

## Upgrade bare

Re-ejecutar el one-liner (conserva `/etc/norix/control-plane.env`, reemplaza binario tras SHA256).

O manual:

```bash
sudo systemctl stop norix-control-plane
# install -m 755 norix-control-plane-linux-… /usr/local/bin/norix-control-plane
sudo systemctl start norix-control-plane
```

## Diff vs modo Docker

| | `--mode bare` | `--mode docker` (default) |
|---|---|---|
| Postgres | Paquete del OS / externo | Contenedor Compose |
| Control plane | Binario + systemd (o `--supervise none`) | Contenedor **sin** `docker.sock` (el **agent** Compose monta el socket; ver [docker-socket.md](../user/docker-socket.md)) |
| Primer worker embebido | No (hasta instalar Docker/agent) | Sí (`localruntime` vía sock) |
| Requiere Docker | No | Sí |
| Integridad binario | SHA256SUMS obligatorio | Imagen build/pull |
