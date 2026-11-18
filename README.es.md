<h1 align="center">Norix</h1>

<p align="center">
  Plano de control PaaS open source y autoalojable.<br>
  Apúntalo a un repo Git o a una imagen — construye, agenda, enruta y termina TLS.<br>
  Dashboard web; agents + edge en tus propios VPS.
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue" alt="License" /></a>
  <a href="VERSION"><img src="https://img.shields.io/badge/version-1.0.0--beta.5-0b7285" alt="Version" /></a>
  <a href="README.md"><img src="https://img.shields.io/badge/lang-English-0b7285" alt="English README" /></a>
</p>

<p align="center">
  <a href="#instalar">Instalar</a> ·
  <a href="docs/user/es/README.md">Guía de usuario</a> ·
  <a href="#cómo-funciona">Cómo funciona</a> ·
  <a href="#para-colaboradores">Colaboradores</a> ·
  <a href="#contribuidores">Contribuidores</a> ·
  <a href="CONTRIBUTING.es.md">Contribuir</a>
</p>

> Traducción orientada a **colaboradores e instaladores** en español. El README canónico en inglés es [`README.md`](README.md).

---

## Instalar

Norix corre en un VPS Linux que controlas. **Un comando** instala el control plane. Elige modo:

| Modo | Comando | ¿Necesita Docker? |
|---|---|---|
| **Docker** (default) | `… \| bash` | Sí — instala Engine si falta; panel + Postgres en Compose; este host puede correr workloads vía `docker.sock` |
| **Bare** | `… \| bash -s -- --mode bare` | No — Postgres del sistema + binario verificado (SHA256 + cosign si hay bundle) + systemd. Añade Docker/agent después para contenedores |

Hasta que exista `get.norix.dev`, el installer se sirve desde este repo de GitHub.

### Verificar binarios del release (bajar → verificar → ejecutar)

El installer bare y `install-agent.sh` comprueban **SHA256SUMS** y, si el release incluye `.cosign.bundle`, **cosign** (fallan si no coincide). Flujo manual:

```bash
TAG=v1.0.0-beta.5
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
ASSET=norix-control-plane-linux-${ARCH}

curl -fsSLO "https://github.com/xiron902224/norix/releases/download/${TAG}/${ASSET}"
curl -fsSLO "https://github.com/xiron902224/norix/releases/download/${TAG}/SHA256SUMS"
curl -fsSLO "https://github.com/xiron902224/norix/releases/download/${TAG}/${ASSET}.cosign.bundle" || true

sha256sum -c SHA256SUMS --ignore-missing   # debe pasar

# Si existe el bundle (y tienes cosign instalado):
cosign verify-blob \
  --bundle "${ASSET}.cosign.bundle" \
  --certificate-identity-regexp 'https://github.com/xiron902224/norix/\.github/workflows/release\.yml@.*' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  "${ASSET}"

sudo install -m 755 "${ASSET}" /usr/local/bin/norix-control-plane
```

Solo labs: `NORIX_SKIP_CHECKSUM=1` (con `NORIX_BINARY_FILE` local) o `NORIX_SKIP_COSIGN=1` / `--skip-cosign`. No usar en producción.

### Modo Docker

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/v1.0.0-beta.5/scripts/get-norix.sh \
  | NORIX_REF=v1.0.0-beta.5 bash
```

### Modo bare (sin Docker en el panel)

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/v1.0.0-beta.5/scripts/get-norix.sh \
  | NORIX_REF=v1.0.0-beta.5 bash -s -- --mode bare
```

Labs / contenedores sin systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/v1.0.0-beta.5/scripts/get-norix.sh \
  | NORIX_REF=v1.0.0-beta.5 bash -s -- --mode bare --supervise none
```

Opcional: `--port 8000`, `--public-url https://norix.example.com`. Re-ejecutar es idempotente.

Guía bare (TLS, upgrades): [`docs/runbooks/BARE_INSTALL.md`](docs/runbooks/BARE_INSTALL.md).

Ambos modos imprimen una URL tipo `http://TU_IP:8000`. Crea el **primer admin** por HTTPS o loopback/túnel SSH (Docker one-click permite bootstrap inseguro hasta poner TLS — [Primeros pasos](docs/user/es/getting-started.md)).

```bash
curl -sf http://127.0.0.1:8000/healthz
```

Tip de integración sin tag: `NORIX_REF=main`.

### Añadir nodos worker

Crea un join token en **Nodes**, luego en cada worker:

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/main/scripts/install-agent.sh \
  | bash -s -- --control-plane https://norix.example.com --token '…'
# auto: systemd en VPS real; nohup en labs sin systemd
```

**Importante:** el control plane bare puede vivir sin Docker, pero los **workers necesitan Docker (o containerd)** para builds Git y contenedores. Si los nodos están `online` pero DeployFromGit falla, revisa `docker_socket` / `containerd_socket`.

---

## Cómo funciona

El CLI de usuario `norix` está escrito en **Rust** (mismo lenguaje que el agent). Guía: [`docs/user/es/cli.md`](docs/user/es/cli.md) · decisión: [ADR-005](docs/adr/ADR-005-rust-cli.md).

1. **Declarar.** Proyecto/workload en el dashboard o con el CLI (imagen, puerto, réplicas, git…).
2. **Build (git).** Un nodo online clona, detecta Dockerfile/Compose, construye.
3. **Schedule.** El control plane coloca réplicas (spread/binpack, zonas, KVM…).
4. **Reconcile.** Agents aplican `run`/`stop` hasta igualar estado deseado.
5. **Ruta + TLS.** Dominios vía Caddy; ACME cuando está configurado.
6. **Operar.** Heartbeats, métricas, backups; cordon/drain antes de reprogramar.

---

## Para colaboradores

| Recurso | Uso |
|---|---|
| [`docs/user/es/`](docs/user/es/README.md) | **Documentación oficial** para usuarios de la plataforma |
| [`CONTRIBUTING.es.md`](CONTRIBUTING.es.md) | Gitflow (`main`), setup local, PRs |
| [`docs/README.md`](docs/README.md) | Mapa de toda la documentación |
| [`ISSUES.md`](ISSUES.md) | Triage del backlog (P0–P3) |
| [`VISION.md`](VISION.md) | Madurez de features + roadmap |
| [`docs/ESTADO_Y_PLAN_V1.md`](docs/ESTADO_Y_PLAN_V1.md) | Estado vs plan S0–S22 |

**Flujo:** tomar un issue → rama desde `main` → PR a `main`.

**Local (resumen):** Postgres (`deploy/norix` compose) → `go run` control-plane `:8080` → Vite `:5173` → (opcional) agent con join token. Detalle en CONTRIBUTING.es.

Buenos puntos de entrada: labels **good first issue** y **priority/P0**.

---

## Contribuidores

<a href="https://github.com/xiron902224/norix/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=xiron902224/norix" alt="Contribuidores de Norix" />
</a>

| GitHub | Rol |
|---|---|
| [@xiron902224](https://github.com/xiron902224) | Maintainer |
| [@Golden007-prog](https://github.com/Golden007-prog) | Colaborador |

Para aparecer: mergear un PR a `main` y/o aceptar invitación en **Settings → Collaborators**. Ver [`CONTRIBUTING.es.md`](CONTRIBUTING.es.md#colaboradores-y-contributors).

---

## Estado

**Beta** (`1.0.0-beta.5`). No es GA. Detalle: [`docs/ESTADO_Y_PLAN_V1.md`](docs/ESTADO_Y_PLAN_V1.md).

---

## Seguridad

Reporta vulnerabilidades **en privado** a los maintainers — no en issues públicos. Política: [`SECURITY.md`](SECURITY.md).

El modo Docker monta el socket de Docker del host en el **agent** (no en el control plane): trata ese VPS como de confianza ([docker-socket.md](docs/user/es/docker-socket.md)).

---

## Licencia

[Apache License 2.0](LICENSE).
