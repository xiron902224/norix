# Propuesta: self-host installer (Norix)

**Estado:** implementado — `scripts/get-norix.sh` con `--mode docker|bare`, preflight, SHA256, `--supervise`, install-state.  
URL pública temporal: `raw.githubusercontent.com/xiron902224/norix/<ref>/scripts/get-norix.sh` (CDN `get.norix.dev` pendiente).

## Objetivo

Un VPS limpio deja Norix usable en **un comando**, sin `go run` / Vite / pasos manuales de DB o binario.

## Modos

| Mode | Stack | Docker |
|---|---|---|
| `docker` (default) | Compose: Postgres + CP (+ sock) | Requerido |
| `bare` | Postgres OS + binario verificado + systemd (o `--supervise none`) | No (solo para el panel) |

## Bare one-click (comportamiento)

1. Preflight: root, arch, puerto, cgroup RAM, systemd (o `--supervise none`)
2. Sync source → `/data/norix/source`
3. Postgres install/start (`systemctl` o `pg_ctlcluster`)
4. Roles/DB idempotentes vía `runuser`/`su` (sin depender de `sudo`)
5. Download release binary + **SHA256SUMS** (fail si falta; no hay build en happy path)
6. `/etc/norix/control-plane.env` (upgrade-safe)
7. Start: systemd unit **o** nohup + pid/log bajo `/var/lib/norix`
8. `healthz` + `/var/lib/norix/install-state.json`

Docs: README Install · [`docs/runbooks/BARE_INSTALL.md`](../runbooks/BARE_INSTALL.md).

## Layout de datos

- Docker: `/data/norix/source` + Compose volumes  
- Bare: `/etc/norix/control-plane.env`, `/var/lib/norix`, binario en `/usr/local/bin`, state JSON

## Pendiente

- Imágenes GHCR (`NORIX_PULL_POLICY=pull` cuando existan)  
- `get.norix.dev` CDN  
- Wizard TLS en UI  
- Auto-enroll agent en bare tras instalar Docker  
- CI smoke bare en VM con systemd  
