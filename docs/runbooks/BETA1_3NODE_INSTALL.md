# Beta — instalar Norix en 3 nodos (servidor real)

Guía operativa para **`v1.0.0-beta.5`**. No es GA: mesh WireGuard, DNS-01 y HA Patroni quedan opt-in.

## Topología recomendada

Tres VPS Linux (amd64 o arm64). El control plane vive en el nodo 1; los tres corren agent + Docker.

```text
                    HTTPS (443) / API
                           │
                     ┌─────▼──────┐
                     │   node-1   │  roles: control-plane + postgres + agent (worker)
                     │  CP :8080  │  public DNS → este host (o LB delante)
                     │  PG :5432  │  (Postgres solo en localhost / private NIC)
                     └─────┬──────┘
              enroll + heartbeat (HTTPS)
              ┌───────────┼───────────┐
              ▼           ▼           ▼
         ┌────────┐  ┌────────┐  ┌────────┐
         │ node-1 │  │ node-2 │  │ node-3 │
         │ agent  │  │ agent  │  │ agent  │
         │ Docker │  │ Docker │  │ Docker │
         └────────┘  └────────┘  └────────┘
```

| Host | Qué corre | Disco / red |
|---|---|---|
| **node-1** | Postgres 16, control plane (UI embed), agent, opcional Caddy | SSD para PG; IP pública o privada alcanzable por node-2/3 |
| **node-2** | agent + Docker | salida a `https://norix.example.com` (CP) |
| **node-3** | agent + Docker | igual |

Alternativa: CP-only en node-1 (sin agent ahí) y solo node-2/3 como workers — entonces tienes 2 workers. Con agent también en node-1 obtienes **3 workers** con 3 máquinas.

### Requisitos por nodo

- Ubuntu 22.04+/Debian 12+ (o equiv.), Docker Engine + Compose v2
- node-1: ≥2 vCPU / 4 GB RAM / 40 GB disco (Postgres + CP)
- workers: ≥2 vCPU / 2 GB RAM (más si builds git)
- Firewall:
  - **Público → node-1:** `443` (y/o `80` para ACME HTTP-01). API puede ir en el mismo host vía reverse proxy.
  - **node-2/3 → node-1:** TCP al puerto del CP (443 o 8080)
  - **No** exponer `5432` a Internet
  - Entre workers: mesh WireGuard aplica por defecto (#48); opt-out `NORIX_APPLY_WIREGUARD=0` (UDP WireGuard)

### Antes de empezar (beta)

1. Tag/release `v1.0.0-beta.5` publicado **o** build from source desde `main`.
2. Dominio apuntando a node-1, p.ej. `norix.example.com`.
3. Tres join tokens (uno por agent) — se crean en la UI tras el bootstrap.

---

## 1. node-1 — Postgres + control plane

### 1.1 Usuario y directorios

```bash
sudo useradd --system --home /var/lib/norix --shell /usr/sbin/nologin norix || true
sudo mkdir -p /etc/norix /var/lib/norix /var/lib/norix/backups
sudo chown -R norix:norix /var/lib/norix
```

### 1.2 Postgres (Compose)

```bash
git clone https://github.com/xiron902224/norix.git /opt/norix
cd /opt/norix
git checkout v1.0.0-beta.5   # o main hasta que exista el tag

sudo mkdir -p /root/.norix
# Genera ~/.norix/.env con passwords aleatorios y levanta postgres (+ pebble/caddy de lab)
./scripts/install.sh up
```

Para **producción**, edita el env de Compose y **quita Pebble** como CA pública. Usa Caddy/nginx delante con Let’s Encrypt real, o DNS-01 en el CP:

```bash
# /etc/norix/control-plane.env (crear en el siguiente paso)
NORIX_ACME_DNS_PROVIDER=cloudflare
NORIX_ACME_EMAIL=ops@example.com
```

Postgres debe escuchar en `127.0.0.1:5432` (el compose del repo ya bindea así).

### 1.3 Binario del control plane

**Desde release (preferido cuando exista el tag):**

```bash
TAG=v1.0.0-beta.5
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -fsSL -O "https://github.com/xiron902224/norix/releases/download/${TAG}/norix-control-plane-linux-${ARCH}"
curl -fsSL -O "https://github.com/xiron902224/norix/releases/download/${TAG}/SHA256SUMS"
sha256sum -c SHA256SUMS --ignore-missing
sudo install -m 755 "norix-control-plane-linux-${ARCH}" /usr/local/bin/norix-control-plane
```

**Desde source (hasta que haya release):**

```bash
cd /opt/norix
make build
sudo install -m 755 control-plane/bin/norix-control-plane /usr/local/bin/norix-control-plane
```

### 1.4 Env de producción

Copia passwords desde `~/.norix/.env` (o `/root/.norix/.env`) y crea `/etc/norix/control-plane.env`:

```bash
sudo tee /etc/norix/control-plane.env >/dev/null <<'EOF'
DATABASE_URL=postgres://norix_app:CHANGE_APP_PASSWORD@127.0.0.1:5432/norix?sslmode=disable
MIGRATION_DATABASE_URL=postgres://norix:CHANGE_OWNER_PASSWORD@127.0.0.1:5432/norix?sslmode=disable
HTTP_ADDR=127.0.0.1:8080
LOG_LEVEL=info
NORIX_PUBLIC_URL=https://norix.example.com
NORIX_WEB_ORIGIN=https://norix.example.com
CORS_ALLOWED_ORIGINS=https://norix.example.com
NORIX_BACKUP_DIR=/var/lib/norix/backups
NORIX_BUILD_SECRETS_KEY=   # openssl rand -hex 32
NORIX_REGISTRY=none
# Opcional beta:
# NORIX_AUTOSCALER=1
# NORIX_APPLY_WIREGUARD=0   # opt-out mesh apply (#48; default is on)
# NORIX_ACME_DNS_PROVIDER=cloudflare
# NORIX_ACME_EMAIL=ops@example.com
EOF
sudo chmod 600 /etc/norix/control-plane.env
sudo chown norix:norix /etc/norix/control-plane.env
```

> Bindear `HTTP_ADDR=127.0.0.1:8080` y terminar TLS en Caddy/nginx hacia ese puerto.

### 1.5 systemd + reverse proxy

```bash
sudo cp /opt/norix/deploy/systemd/norix-control-plane.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now norix-control-plane
curl -sf http://127.0.0.1:8080/healthz && curl -sf http://127.0.0.1:8080/status
```

Ejemplo mínimo Caddy (en node-1):

```caddyfile
norix.example.com {
  reverse_proxy 127.0.0.1:8080
}
```

Abre `https://norix.example.com`, crea el admin inicial (bootstrap), org y un proyecto de prueba.

---

## 2. Agents en node-1, node-2 y node-3

En la UI: **Nodes → Create join token** (un token **por** nodo; son one-time).

En **cada** worker (incluye node-1 si quieres 3 schedulable workers):

```bash
# Docker obligatorio
sudo apt-get update && sudo apt-get install -y docker.io
sudo systemctl enable --now docker

# Binario: release o source
TAG=v1.0.0-beta.5
# … descargar norix-agent-linux-$ARCH + verificar SHA256SUMS …
# o: cd /opt/norix && cargo +1.88.0 build --release -C agent && sudo install -m 755 agent/target/release/norix-agent /usr/local/bin/norix-agent

sudo mkdir -p /var/lib/norix-agent
sudo tee /var/lib/norix-agent/agent.env >/dev/null <<EOF
NORIX_CONTROL_PLANE_URL=https://norix.example.com
NORIX_JOIN_TOKEN=PASTE_TOKEN_HERE
NORIX_NODE_NAME=$(hostname -s)
NORIX_AGENT_WORKDIR=/var/lib/norix-agent
NORIX_HEARTBEAT_SECS=10
NORIX_AGENT_LOG_LEVEL=info
EOF
sudo chmod 600 /var/lib/norix-agent/agent.env

# Alternativa one-click (systemd en VPS; nohup si no hay systemd):
# ./scripts/install-agent.sh --control-plane https://norix.example.com \
#   --token '…' --name "$(hostname -s)" --version "$TAG"
# Labs sin systemd: añade --supervise none (o deja auto).
# Solo instalar sin arrancar: --no-start

sudo cp /opt/norix/deploy/systemd/norix-agent.service /etc/systemd/system/
# Si el repo no está en el worker, copia solo el unit desde node-1.
# (install-agent.sh sin --no-start ya escribe/activa el unit cuando hay systemd)
sudo systemctl daemon-reload
sudo systemctl enable --now norix-agent
sudo systemctl status norix-agent --no-pager
```

Tras el primer enroll exitoso, **borra `NORIX_JOIN_TOKEN`** del `agent.env` y `systemctl restart norix-agent` (el estado queda en el workdir).

Comprueba en UI **Nodes**: tres nodos `online`. Smoke:

```bash
norix-agent --check
```

En un nodo que solo aloja el control plane bare no hace falta Docker. En los
workers, `online` confirma los heartbeats. Antes de intentar un build Git,
[verifica el runtime del worker](./BARE_INSTALL.md#un-worker-online-no-acepta-builds-git).

---

## 3. Validar el cluster (DoD beta multi-nodo)

1. Proyecto → workload imagen `nginx:alpine`, **replica_count = 3**, strategy **spread**.
2. Deploy → en &lt; ~1 min deben aparecer contenedores en los 3 agents (`docker ps --filter label=norix.workload_id`).
3. Cordon node-2 → nuevas réplicas no caen ahí.
4. Drain node-3 → workloads emigran; matar agent en node-3 → reschedule &lt; ~60s hacia nodos sanos (S7).
5. `curl -sf https://norix.example.com/healthz` y `/status`.

### Mesh WireGuard (#48)

Por defecto el CP marca `DesiredNetworkPlan.apply=true`. Los agents aplican `wg-quick` + `nft` si las herramientas existen; si fallan, revierten a la conf anterior. Para solo escribir configs:

```bash
NORIX_APPLY_WIREGUARD=0
```

Abre UDP WireGuard entre nodos y verifica `wg show`. El scheduler multi-nodo funciona igual sin mesh.

---

## 4. Operación diaria

| Tarea | Cómo |
|---|---|
| Backup PG | `./scripts/backup-postgres.sh` o Ops UI → [`S7_OPERATIONS.md`](./S7_OPERATIONS.md) |
| Upgrade CP/agent | bajar assets del nuevo tag, `sha256sum -c`, replace binaries, `systemctl restart …` |
| Alertas | `NORIX_ALERT_WEBHOOK_URL` en CP |
| Logs | `journalctl -u norix-control-plane -f` / `journalctl -u norix-agent -f` |

---

## 5. Límites honestos de Beta

- Sin tag/release aún → instalar from source desde `main` (`make build` / `cargo build --release`).
- Tip de integración: **`main`** (rama `develop` retirada). Preferir tag `v1.0.0-beta.5` cuando exista ([#62](https://github.com/xiron902224/norix/issues/62)).
- Workers necesitan **Docker/containerd** para builds Git ([#60](https://github.com/xiron902224/norix/issues/60), [#66](https://github.com/xiron902224/norix/issues/66)).
- Mesh: apply on by default (#48); opt-out `=0`. micro-VM / DNS-01: opt-in. Patroni: smoke `#52`. Autoscaler: GA detrás de `NORIX_AUTOSCALER=1` (#49).
- El `local` node embebido del CP puede aparecer en inventory si Docker está en node-1; el scheduler prioriza agents remotos. Puedes cordonar el local en UI si no quieres workloads ahí.
- No hay installer “one-click” multi-nodo: este runbook + systemd es el camino soportado en beta.

## Checklist pre-anuncio beta

- [ ] Tres agents `online`
- [ ] Deploy spread 3/3 running
- [ ] Cordon/drain smoke
- [ ] Backup/restore PG de prueba
- [ ] TLS público en `NORIX_PUBLIC_URL`
- [ ] Join tokens rotados / quitados de `agent.env`
