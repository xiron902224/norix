# Propuesta: self-host tipo Coolify / OpenChiip

**Estado:** **MVP implementado** en árbol (2026-11-15) — `scripts/get-norix.sh` + `deploy/norix/docker-compose.prod.yml` + `control-plane/Dockerfile.prod`.  
URL pública temporal: `raw.githubusercontent.com/xiron902224/norix/<ref>/scripts/get-norix.sh` (hasta `get.norix.dev`).  
Pendiente beta tag: imágenes GHCR (hoy `NORIX_PULL_POLICY=build`), auto-enroll agent, Caddy 80/443.  
**Objetivo:** que un VPS limpio quede con Norix usable en **un comando**, sin correr `go run` / Vite a mano.

---

## 1. Qué hacen ellos (y qué queremos copiar)

| | **Coolify** | **OpenChiip Harness** | **Norix hoy** |
|---|---|---|---|
| One-liner | `curl …/install.sh \| bash` | `curl https://get.openchiip.com \| bash` | No (clone + compose parcial + binarios a mano) |
| Empaqueta | Docker Compose: panel + PG + Redis + proxy | venv `/opt/…` + systemd (o compose) | `install.sh` solo PG/edge; CP en profile `full` sin imagen publicada |
| Primer servidor | Panel **y** primer destino de apps (docker.sock) | Proceso único local | CP y agent separados; agent no auto-enroll |
| Datos | `/data/coolify/{source,ssh,backups,…}` | `/opt/openchiip-harness` | `~/.norix` + repo path |
| Upgrade | `upgrade.sh` pull imágenes | pip/systemd | Manual |
| Nodos extra | SSH desde el panel | Modo platform WS | Join token + `install-agent.sh` |

**Para Norix (PaaS de contenedores) el análogo correcto es Coolify/Dokploy/CapRover**, no el modelo “un solo binario AI” de OpenChiip.  
De OpenChiip sí conviene copiar: dominio `get.*`, script auditable, y salida clara “abre http://IP:PORT”.

---

## 2. Experiencia objetivo (Beta)

### Instalar el control plane (all-in-one)

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/v1.0.0-beta.1/scripts/get-norix.sh | bash
# alias futuro: https://get.norix.dev | bash
```

Al terminar:

```text
Norix is ready.
  UI / API:  http://YOUR_IP:8000
  Health:    http://YOUR_IP:8000/healthz
  Data dir:  /data/norix
Next: open the URL → create admin → Nodes (this host already enrolled).
Add workers: curl … | bash -s -- --agent --token TOKEN --url https://norix.example.com
```

### Añadir nodos 2 y 3

```bash
curl -fsSL …/get-norix.sh | bash -s -- --agent \
  --url https://norix.example.com \
  --token 'JOIN_TOKEN'
```

Misma familia de script; modo `--agent` solo instala Docker (si falta) + agent + systemd/compose sidecar.

---

## 3. Arquitectura recomendada: “Coolify-shaped”

```text
                    ┌──────────────────────────────────────────┐
  :80/:443 apps     │  VPS #1  /data/norix                     │
  :8000 panel ───►  │                                          │
                    │  compose (norix stack):                   │
                    │    postgres          (volume)             │
                    │    control-plane     (image GHCR, UI emb) │
                    │    caddy             (80/443 → apps+panel)│
                    │    agent             (docker.sock mount)  │
                    │                                          │
                    │  Primer worker = este host (auto-enroll)  │
                    └──────────────────┬───────────────────────┘
                                       │ heartbeat / enroll
                         ┌─────────────┴─────────────┐
                         ▼                           ▼
                   VPS #2 agent                 VPS #3 agent
```

### Decisiones de producto

1. **Un solo comando instala “todo lo del panel”** en el primer servidor: Postgres + CP (UI embed) + agent local + (fase 2) proxy 80/443.  
   No pedir `git clone` + `make build` al usuario final.
2. **Imágenes publicadas** (`ghcr.io/xiron902224/norix-control-plane:1.0.0-beta.1`, `…/norix-agent:…`). Sin imagen en registry, el one-liner no escala (habría que build en el VPS = lento y frágil).
3. **Primer nodo = destino de workloads** (Coolify). El agent monta `/var/run/docker.sock`. El `localruntime` embebido del CP se puede dejar como fallback o cordonar en UI; idealmente el installer genera un join token interno y enrolla el agent del host.
4. **Nodos extra = agent only** (fortaleza Norix vs Coolify-SSH). No hace falta SSH root desde el panel para beta.
5. **Layout de datos fijo:** `/data/norix/{source,env,backups,ssh,agent}` — upgrade idempotente sin depender del home del usuario.
6. **Puertos beta.1:** panel en **`:8000`** público (como Coolify `:8000` / Dokploy `:3000`). TLS del panel en fase 2 (dominio en UI). Apps pueden usar host ports o Caddy cuando edge esté cableado al compose prod.
7. **Root/sudo** requerido (igual que Coolify). Fresh VPS recomendado.

---

## 4. Qué falta hoy en el repo (gaps)

| Gap | Impacto |
|---|---|
| No hay `get-norix.sh` standalone (CDN/raw) | No hay one-liner |
| Compose `full` hace `build:` local, bind `127.0.0.1:8080` | No sirve “abre la IP del VPS” |
| Release workflow no publica imágenes GHCR | No hay `docker pull` versionado |
| Agent no va en el mismo compose del CP | Primer servidor no despliega apps sin paso manual |
| Auto-enroll del agent del host | Usuario debe crear join token a mano |
| `install.sh` actual | Solo deps de lab (PG+Pebble), no producto |

El runbook de 3 nodos (`docs/runbooks/BETA1_3NODE_INSTALL.md`) sigue válido como **modo avanzado/systemd**; el one-liner es el **modo default self-host**.

---

## 5. Diseño técnico del installer

### `scripts/get-norix.sh` (entrypoint público)

Responsabilidades (orden Coolify-like):

1. Detectar root/sudo, amd64|arm64, OS.
2. Instalar Docker Engine si falta (`get.docker.com` o paquetes distro — documentar trust).
3. Crear `/data/norix/{source,env,backups,agent}`.
4. Generar `.env` con secrets (`openssl rand`): DB passwords, `NORIX_BUILD_SECRETS_KEY`, `NORIX_BOOTSTRAP_JOIN_TOKEN`.
5. Descargar `docker-compose.prod.yml` + pin de versión (`NORIX_VERSION=1.0.0-beta.1`).
6. `docker compose pull && up -d`.
7. Esperar `/healthz`.
8. Imprimir URL (`http://$(curl -s ifconfig.me):8000` o IP privada detectada).
9. Modos: default = all-in-one; `--agent` = solo worker; `--upgrade`; `--uninstall`.

### `deploy/norix/docker-compose.prod.yml` (nuevo)

```yaml
# Esqueleto propuesto (no committed como final aún)
services:
  postgres: ...
  control-plane:
    image: ghcr.io/xiron902224/norix-control-plane:${NORIX_VERSION}
    ports: ["8000:8080"]          # panel público en beta
    volumes: []                   # sin docker.sock en el CP
    environment:
      DATABASE_URL: ...
      NORIX_PUBLIC_URL: ${NORIX_PUBLIC_URL:-http://0.0.0.0:8000}
  agent:
    image: ghcr.io/xiron902224/norix-agent:${NORIX_VERSION}
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /data/norix/agent:/var/lib/norix-agent
    environment:
      NORIX_CONTROL_PLANE_URL: http://control-plane:8080
      NORIX_JOIN_TOKEN: ${NORIX_BOOTSTRAP_JOIN_TOKEN}
  # caddy: fase 2 — 80/443 + ACME real (sin Pebble)
```

### Release / CI (imprescindible para el one-liner)

Extender `.github/workflows/release.yml` (o job en tag `v*`):

- Buildx multi-arch → `ghcr.io/…/norix-control-plane:1.0.0-beta.1` + `:beta` + digest
- Igual para `norix-agent`
- Adjuntar `get-norix.sh` + `docker-compose.prod.yml` como assets del Release
- Cosign ya previsto para binarios; firmar también imágenes (opcional beta)

### Auto-enroll (detalle CP)

Opción A (simple beta): env `NORIX_BOOTSTRAP_JOIN_TOKEN` pre-insertado en DB en migrate/boot si no hay nodos.  
Opción B: endpoint interno one-shot `POST /internal/bootstrap-node` solo desde red docker.  
Recomendación: **A** para beta.1 (menos superficie HTTP).

---

## 6. Fases de entrega

| Fase | Entrega | Criterio de hecho | ¿Beta.1? |
|---|---|---|---|
| **A** | `docker-compose.prod.yml` + imágenes en GHCR en tag | `docker compose up` en VPS limpio → UI en `:8000` + agent online | **Must** |
| **B** | `get-norix.sh` one-liner (raw GitHub o release asset) | `curl \| bash` sin clonar repo | **Must** |
| **C** | `--agent` en el mismo script + docs README “Self-host” | 3 nodos con 2 curl | Should |
| **D** | Caddy 80/443 + wizard dominio/HTTPS en UI | Panel en `https://norix.example.com` | Should / GA |
| **E** | `get.norix.dev` CDN + auto-upgrade channel | Paridad Coolify polish | Post-beta |

**Recomendación:** no anunciar `v1.0.0-beta.1` hasta cerrar **A+B**. El runbook systemd queda como path “power user”.

---

## 7. Alternativa descartada (por ahora)

**Solo binarios + systemd** (estilo OpenChiip `/opt` + unit): útil, ya esbozado en `deploy/systemd/`, pero:

- El usuario sigue instalando Postgres aparte.
- No hay “un comando = stack completo”.
- Coolify/Dokploy ganaron la UX mental del mercado PaaS self-host con Compose.

Usar systemd como **fallback** documentado; el default marketing = Docker one-liner.

---

## 8. Riesgos

| Riesgo | Mitigación |
|---|---|
| `curl \| bash` + trust en get.docker.com | Script en repo; pin de versión; documentar `--skip-docker-install` |
| Docker.sock = root equivalente | Documentar “fresh VPS”; agent user/root explícito |
| Panel en `:8000` HTTP | Aviso fuerte; fase D HTTPS; firewall guide |
| Imagen CP sin sock pero localruntime en host | En compose, CP sin Docker → solo agent reconcilia (evitar doble runtime) |
| GHCR permisos / packs | Workflow `packages: write`; imagen pública read |

---

## 9. Decisión pedida

1. **¿Default self-host = Compose all-in-one (Coolify-like)?** → propuesta: **sí**.  
2. **¿Beta.1 bloqueada a fases A+B (imágenes + get-norix.sh)?** → propuesta: **sí**.  
3. **¿Panel port 8000 o 8080?** → propuesta: **8000** (evita choque mental con APIs internas; alinea con Coolify).  
4. **¿Implementar ya la fase A (compose.prod + Dockerfile agent + job GHCR)?**

Cuando confirmes 1–4, el siguiente PR concreto sería:

1. `deploy/norix/docker-compose.prod.yml`  
2. `agent/Dockerfile`  
3. Release → push GHCR  
4. `scripts/get-norix.sh`  
5. README: sección **Self-host** arriba del Quick Start local  
6. Mantener el runbook 3 nodos como “Advanced”
