# Norix — Estado vs plan S0–S22

**Fecha:** 2026-11-15  
**Plan canónico:** [`docs/PLAN_SPRINTS_S0_S22.md`](./PLAN_SPRINTS_S0_S22.md)  
**Visión / backlog:** [`VISION.md`](../VISION.md) · [`ISSUES.md`](../ISSUES.md) (55 issues abiertos en GitHub)  
**Hito código:** **Beta** (`VERSION` = `1.0.0-beta.3`) — superficie S0–S22 + installer docker/bare + agent supervise, **no GA**  
**Hito release:** tag `v1.0.0-beta.3` (tras merge); ya publicados `v1.0.0-beta.1` y `v1.0.0-beta.2`

---

## 1. Dónde estamos (honesto)

| | |
|---|---|
| **Fase plan** | S0–S22 con checklist de código; anuncio GA **aplazado** a favor de beta pública |
| **Código** | Tip en `develop` / `main` alineados; rama de release `chore/1.0.0-beta.3` |
| **Remoto** | Pre-releases GitHub: `v1.0.0-beta.1`, `v1.0.0-beta.2`; siguiente `v1.0.0-beta.3` |
| **CI** | Baseline `buf breaking` corregida; seguir vigilando verdes en PRs |
| **Backlog** | 55 issues: **10 P0**, 14 P1, 19 P2, 12 P3 — Stage 0 (trust) **aún abierto** |

### Por qué Beta y no GA

- **Alpha** (plan S6) ya quedó atrás: hay multi-nodo, DBs, sandboxes, HA/HPA, SDKs.
- **GA 1.0** es prematuro: Stage 0 de seguridad incompleto, E2E multi-nodo no está en CI, varias features ops gated sin smoke firmado.
- **Beta** (`1.0.0-beta.x`) comunica “track 1.0 usable, API/ops aún en endurecimiento”.

### Qué aporta beta.3

- Agent installer: `--supervise auto|systemd|none` (no bloquea la terminal en labs).
- Bare installer endurecido (SHA256, supervise, install-state).
- Docs + `ISSUES.md` / `VISION.md` alineados con el board de GitHub.

---

## 2. Tablero S0–S22

| Sprint | Estado doc | Notas código |
|---|---|---|
| S0 | Parcial | Spikes KVM |
| S1–S6 | Cerrado | Alpha un nodo |
| S7–S10 | Cerrado | Multi-nodo / mesh / obs / DB |
| S11–S14 | Cerrado | micro-VM gated, sandboxes, CLI/SDK, previews+TTL |
| S15–S18 | Cerrado | Wasm + teams + HA leases **cableados** + HPA **apply** |
| S19–S21 | Cerrado | SBOM, k6/chaos scaffold, docs/upgrade; telemetría opt-in |
| S22 | Código listo / release abierto | Cosign/multi-arch workflow; tags **beta** en curso (GA después) |

---

## 3. Implementado en código

| Área | Estado | Evidencia |
|---|---|---|
| Auth + RLS | Listo | migraciones + CI RLS |
| TOTP real + enforce | Listo | `internal/db/totp.go`, mig `000022` |
| OIDC login | Listo | `/auth/oidc/start\|callback` (`api/oidc.go`); guía [`AUTH_OIDC_TOTP.md`](./AUTH_OIDC_TOTP.md) |
| Deploy imagen/git | Listo | localruntime + agent |
| Compose stacks | Listo (endurecer en beta) | `AnalyzeCompose`/`DeployCompose`, mig `000020` |
| Edge TLS local | Listo | Caddy + Pebble |
| DNS-01 ACME | Listo (env) | lego `NORIX_ACME_DNS_PROVIDER` + email |
| WireGuard apply | Listo (env) | `NORIX_APPLY_WIREGUARD=1` |
| HA leases | Listo | `ha.RunElection` en `cmd/server` |
| Autoscaler apply | Listo (env) | `NORIX_AUTOSCALER=1` → `replica_count` |
| Managed DB backup | Listo | dump local + S3 opcional (`managed_backup.go`) |
| Sandbox exec + warm pool | Listo | docker exec / `EnsureSandboxWarmPool` |
| Preview TTL/GC | Listo | mig `000021` |
| Wasm + WASI HTTP | Listo (env) | agent `NORIX_WASM_SERVE=1` |
| Micro-VM | Listo (hardware) | `agent/src/microvm.rs` + KVM/kernel/rootfs |
| SDKs | Listo | `sdk/typescript`, `sdk/python`, `sdk/go` |
| Status + telemetry | Listo | `GET /status`, `NORIX_TELEMETRY=1` |
| Patroni | Listo (compose) | `deploy/postgresql/patroni/` + `deploy/norix/docker-compose.patroni.yml` |
| Installer docker/bare | Listo (beta) | `scripts/get-norix.sh` + SHA256 + `--supervise` |
| Agent join | Listo (beta) | `scripts/install-agent.sh` + systemd/nohup |
| Release multi-arch+cosign | Workflow listo | `.github/workflows/release.yml` |
| License / contrib | Listo | `LICENSE` (Apache-2.0), `CONTRIBUTING.md`, `ISSUES.md` |

### Flags de ops (feature on host)

| Env | Efecto |
|---|---|
| `NORIX_APPLY_WIREGUARD=1` | wg-quick + nft |
| `NORIX_MICROVM_KERNEL` / `NORIX_MICROVM_ROOTFS` | boot Cloud Hypervisor |
| `NORIX_AUTOSCALER=1` | HPA loop |
| `NORIX_ACME_DNS_PROVIDER` + `NORIX_ACME_EMAIL` | DNS-01 |
| `NORIX_BACKUP_S3_BUCKET` (+ creds/endpoint) | managed DB → S3 |
| `NORIX_TELEMETRY=1` (+ `NORIX_TELEMETRY_URL`) | heartbeat anónimo |
| `NORIX_WASM_SERVE=1` | `wasmtime serve` |

---

## 4. Qué queda (checklist operativo)

### Must antes / al publicar `v1.0.0-beta.3`

- [x] Documentar supervise agent + bare labs
- [x] Publicar triage de issues (`ISSUES.md`)
- [ ] **PR verde** a `develop`
- [ ] **Tag** `v1.0.0-beta.3` y disparar workflow Release
- [ ] Smoke: install agent con nohup en lab + nodo `online` en UI
- [ ] (Should) Smoke **3 nodos** según [`runbooks/BETA1_3NODE_INSTALL.md`](./runbooks/BETA1_3NODE_INSTALL.md)

### Must Stage 0 (antes de rc) — ver [`ISSUES.md`](../ISSUES.md)

- [ ] P0 seguridad: CORS (#9), secrets fail-closed (#7), webhook HMAC (#3), SSRF (#4), Docker socket docs (#10), vuln scan CI (#12), …
- [ ] Docs maturity honestos en README vs tabla VISION (#30)

### Must antes de GA `v1.0.0`

- [ ] Stage 0 cerrado + smoke E2E estable
- [ ] Multi-nodo E2E en CI (#37) + upgrade N-1 → N (#38)
- [ ] Docs de upgrade probadas

### Should (validación en host)

- [ ] 2 nodos con `NORIX_APPLY_WIREGUARD=1` (`wg show`)
- [ ] Micro-VM en máquina con `/dev/kvm` + artefacts
- [ ] DNS-01 con provider real (Cloudflare/Route53/…)
- [ ] Patroni failover smoke (`deploy/postgresql/patroni`)
- [ ] Backup managed DB a S3-compatible

---

## 5. Smoke local

```bash
curl -s http://127.0.0.1:8080/healthz   # ok
curl -s http://127.0.0.1:8080/status    # JSON components
```

Quick Start + producción: README raíz. Contribución: [`CONTRIBUTING.md`](../CONTRIBUTING.md).  
Self-host: [`scripts/get-norix.sh`](../scripts/get-norix.sh) (`--mode docker|bare`) · bare [`runbooks/BARE_INSTALL.md`](./runbooks/BARE_INSTALL.md).  
Agent: [`scripts/install-agent.sh`](../scripts/install-agent.sh) (`--supervise auto|systemd|none`).
