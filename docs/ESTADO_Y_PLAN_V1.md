# Norix — Estado vs plan S0–S22

**Fecha:** 2026-11-16  
**Plan canónico:** [`docs/PLAN_SPRINTS_S0_S22.md`](./PLAN_SPRINTS_S0_S22.md)  
**Visión / backlog:** [`VISION.md`](../VISION.md) · [`ISSUES.md`](../ISSUES.md)  
**Changelog:** [`CHANGELOG.md`](../CHANGELOG.md) · **versión en árbol:** [`VERSION`](../VERSION) = `1.0.0-beta.4`  
**Guía de usuario:** [`docs/user/`](./user/)  
**Colaborar:** [`CONTRIBUTING.md`](../CONTRIBUTING.md) · [`CONTRIBUTING.es.md`](../CONTRIBUTING.es.md) · [`README.es.md`](../README.es.md)  
**Hito código:** **Beta** — **no GA**  
**Hito release:** pre-release `v1.0.0-beta.5` desde `main`; tracking [#82](https://github.com/xiron902224/norix/issues/82)

---

## 1. Dónde estamos (honesto)

| | |
|---|---|
| **Fase plan** | S0–S22 en código; GA aplazado |
| **Gitflow** | **Solo `main`**: issue → rama desde `main` → PR a `main`. `develop` retirada |
| **Remoto** | Default branch = `main`; tags pre-release `v1.0.0-beta.1`…`beta.5` |
| **CI** | Protecciones en `main`; alinear nombres de checks ([#67](https://github.com/xiron902224/norix/issues/67)); vuln scans + upgrade smoke en CI |
| **Backlog** | beta.5 shipped; Stage 1+ diferido |

### Por qué Beta y no GA

- Superficie S0–S22 amplia, Stage 0 mínimo para beta.5 cerrado; residual Stage 0 sigue abierto.
- E2E multi-nodo aún no está en CI; features ops gated / Experimental.
- Beta comunica “usable en happy path, aún endureciendo”.

### Qué aporta beta.4 (ver `CHANGELOG`)

- Versión real en CP / UI / agent (`/status`, badge bajo el logo).
- Update / uninstall CLI para CP + agent; docs oficiales `docs/user/`.
- Gitflow main-only; pins de installer/docs a `v1.0.0-beta.4`.

### Qué aporta beta.5 (ver `CHANGELOG`)

- Deploy honesty (#59/#60/#63), Stage 0 trust slice (#7/#9/#12/#3/#11).
- Docs/status/TLS/troubleshooting/repo metadata (#30/#33/#31/#32/#67).
- Upgrade smoke N-1→N, deploy progress UI, first-run onboarding (#38/#36/#19).

---

## 2. Maturity matrix (canónica)

Misma taxonomía que [`VISION.md` §3](../VISION.md):

| Maturity | Meaning |
|---|---|
| **Stable** | Implemented, tested, documented |
| **Beta** | Works on the happy path; gaps or limited tests |
| **Experimental** | Behind a flag / partially wired; do not rely on it |
| **Planned** | Not in code yet |

| Capability | Maturity | Notes |
|---|---|---|
| Deploy from image | Stable | Docker / nerdctl via agent |
| Deploy from Git (Dockerfile / Containerfile) | Beta | Needs worker with Docker/containerd; clearer unschedulable errors in beta.5 |
| Compose stack deploy | Beta | Parsed into N workloads |
| Scheduler, cordon / drain, desired / observed | Stable | Unit tests; multi-node E2E not in CI |
| Multi-tenancy (RLS), RBAC, OIDC, TOTP, audit | Stable | RLS isolation test in CI |
| Dashboard | Beta | Limited E2E |
| Installer (docker / bare), join tokens | Beta | SHA256 verified; agent `--supervise` |
| CP / agent check · update · uninstall | Beta | Documented in `docs/user/` + `UPDATE_AND_UNINSTALL.md` |
| Git webhooks → build | Beta | HMAC signature hardening in beta.5 (#3) |
| Domains + Caddy edge | Beta | Route health + rollback |
| TLS / ACME | Experimental | DNS-01 exists; CP serves plain HTTP by default |
| Preview environments | Experimental | Stub pipeline; webhook secret required (#11) |
| Managed databases | Experimental | Records/backups; engine not provisioned |
| Autoscaler | Experimental | `NORIX_AUTOSCALER=1` |
| WireGuard mesh | Experimental | Dry-run by default |
| Wasm (Wasmtime) | Experimental | `NORIX_WASM_SERVE=1`; not production-ready |
| Micro-VM (Cloud Hypervisor) | Experimental | Needs KVM + artifacts — **not** “ready” |
| Micro-VM (Firecracker) | Planned | Enum only |
| App rollbacks / rolling updates / volumes / buildpacks / catalog | Planned | |
| CLI / SDKs | Experimental | Minimal |
| User API tokens (PAT) | Planned | |

> **No etiquetar Experimental como “listo”.** Wasm y micro-VM están cableados detrás de flags; no son GA.

---

## 3. Tablero S0–S22

| Sprint | Estado doc | Notas |
|---|---|---|
| S0 | Parcial | Spikes KVM |
| S1–S21 | Cerrado (código) | Ver `docs/SPRINT_*` |
| S22 | Release abierto | Tags beta en curso |

Flags útiles: `NORIX_APPLY_WIREGUARD`, `NORIX_MICROVM_*`, `NORIX_AUTOSCALER`, `NORIX_ACME_*`, `NORIX_BACKUP_S3_*`, `NORIX_TELEMETRY`, `NORIX_WASM_SERVE`, `NORIX_ALLOW_INSECURE_DEV_SECRETS`, `NORIX_CORS_ALLOW_WILDCARD`.

---

## 4. Checklist operativo

### `v1.0.0-beta.4`

- [x] Versión real CP/UI/agent + update/uninstall path
- [x] Docs usuario `docs/user/`
- [x] Tag / release assets según workflow de release

### `v1.0.0-beta.5`

- [x] Waves A–D del milestone (#82)
- [ ] Smoke post-tag: install → agent con Docker → deploy → check/update CLI → `/status` = UI

### Stage 0 residual (rc)

Ver [`ISSUES.md`](../ISSUES.md): #4–#6, #8, #10, #13 y vecinos — **no** bloquean beta.5.

### Antes de GA `v1.0.0`

Stage 0 cerrado · E2E multi-nodo CI (#37) · upgrade N-1→N (#38) · docs upgrade probadas.

---

## 5. Smoke local

```bash
curl -s http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8080/status
```

Installer: `scripts/get-norix.sh` (`NORIX_REF=main` o tag) · Agent: `scripts/install-agent.sh`.
