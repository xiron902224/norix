# Norix — Estado vs plan S0–S22

**Fecha:** 2026-11-15  
**Plan canónico:** [`docs/PLAN_SPRINTS_S0_S22.md`](./PLAN_SPRINTS_S0_S22.md)  
**Visión / backlog:** [`VISION.md`](../VISION.md) · [`ISSUES.md`](../ISSUES.md)  
**Colaborar:** [`CONTRIBUTING.md`](../CONTRIBUTING.md) · [`CONTRIBUTING.es.md`](../CONTRIBUTING.es.md) · [`README.es.md`](../README.es.md)  
**Hito código:** **Beta** (`VERSION` = `1.0.0-beta.4`) — **no GA**  
**Hito release:** tag `v1.0.0-beta.4` desde `main` ([#62](https://github.com/xiron902224/norix/issues/62))

---

## 1. Dónde estamos (honesto)

| | |
|---|---|
| **Fase plan** | S0–S22 en código; GA aplazado |
| **Gitflow** | **Solo `main`**: issue → rama desde `main` → PR a `main`. `develop` retirada |
| **Remoto** | Default branch = `main`; pre-releases `v1.0.0-beta.1` / `.2`; falta publicar `.3` |
| **CI** | Protecciones en `main`; alinear nombres de checks ([#67](https://github.com/xiron902224/norix/issues/67)) |
| **Backlog** | ~60+ issues; Stage 0 abierto; gaps de lab: [#59](https://github.com/xiron902224/norix/issues/59)–[#60](https://github.com/xiron902224/norix/issues/60) |

### Por qué Beta y no GA

- Superficie S0–S22 amplia, pero Stage 0 de seguridad incompleto.
- E2E multi-nodo aún no está en CI; features ops gated.
- Beta comunica “usable, aún endureciendo”.

### Qué aporta beta.3 (+ docs main)

- Agent `--supervise` systemd/nohup; bare installer SHA256/supervise.
- Docs + triage GitHub; README/CONTRIBUTING en español.
- Gitflow simplificado a `main`.

---

## 2. Tablero S0–S22

| Sprint | Estado doc | Notas |
|---|---|---|
| S0 | Parcial | Spikes KVM |
| S1–S21 | Cerrado (código) | Ver cierres en `docs/SPRINT_*` |
| S22 | Release abierto | Tags beta en curso |

---

## 3. Implementado (resumen)

Auth/RLS, TOTP, OIDC, deploy imagen/git/compose, edge/ACME DNS-01 (env), WG apply (env), HA leases, autoscaler (env), managed DB backup, sandboxes, previews TTL, Wasm/micro-VM (gated), SDKs, `/status`, Patroni compose, installer docker/bare, agent join con supervise.

Flags: `NORIX_APPLY_WIREGUARD`, `NORIX_MICROVM_*`, `NORIX_AUTOSCALER`, `NORIX_ACME_*`, `NORIX_BACKUP_S3_*`, `NORIX_TELEMETRY`, `NORIX_WASM_SERVE`.

---

## 4. Checklist operativo

### Al publicar / estabilizar `v1.0.0-beta.4`

- [x] Agent supervise + docs bare/labs
- [x] `ISSUES.md` / `VISION.md` / ES docs / gitflow `main`
- [ ] PR verde a **`main`**
- [ ] Tag `v1.0.0-beta.4` ([#62](https://github.com/xiron902224/norix/issues/62))
- [ ] Smoke agent + Docker en worker ([#60](https://github.com/xiron902224/norix/issues/60), [#66](https://github.com/xiron902224/norix/issues/66))

### Stage 0 (antes de rc)

Ver [`ISSUES.md`](../ISSUES.md): #3–#12, #30, #33, más #59/#60.

### Antes de GA `v1.0.0`

Stage 0 cerrado · E2E multi-nodo CI (#37) · upgrade N-1→N (#38) · docs upgrade probadas.

---

## 5. Smoke local

```bash
curl -s http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8080/status
```

Installer: `scripts/get-norix.sh` (`NORIX_REF=main` o tag) · Agent: `scripts/install-agent.sh`.
