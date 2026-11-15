# Propuesta: self-host installer (Norix)

**Estado:** implementado en árbol — `scripts/get-norix.sh` con `--mode docker|bare`.  
URL pública temporal: `raw.githubusercontent.com/xiron902224/norix/<ref>/scripts/get-norix.sh`.

## Objetivo

Un VPS limpio deja Norix usable en un comando, sin `go run` / Vite.

## Modos

| Mode | Stack | Docker |
|---|---|---|
| `docker` (default) | Compose: Postgres + CP (+ sock) | Requerido |
| `bare` | Postgres OS + binario + systemd | No (solo para el panel) |

Docs: README Install · [`docs/runbooks/BARE_INSTALL.md`](../runbooks/BARE_INSTALL.md).

## Layout de datos

- Docker: `/data/norix/source` + Compose volumes  
- Bare: `/etc/norix/control-plane.env`, `/var/lib/norix`, binario en `/usr/local/bin`

## Pendiente

- Imágenes GHCR (`NORIX_PULL_POLICY=pull` cuando existan)  
- `get.norix.dev` CDN  
- Wizard TLS en UI  
- Auto-enroll agent en bare tras instalar Docker  
