# Norix — Estado vs plan S0–S22

**Fecha:** 2026-11-15  
**Plan canónico:** [`docs/PLAN_SPRINTS_S0_S22.md`](./PLAN_SPRINTS_S0_S22.md)  
**Hito código:** **GA 1.0 implementado** (`VERSION` = 1.0.0)  
**Hito release:** **pendiente** — working tree sin merge/tag `v1.0.0` en remoto

---

## 1. Dónde estamos (honesto)

| | |
|---|---|
| **Fase plan** | S0–S22 marcados cerrados en docs de sprint |
| **Código local** | Superficie GA 1.0 presente en working tree (`develop` + ~140 archivos dirty) |
| **Remoto** | `origin/develop` aún en commits GA “surface” previos; **sin tag `v*`** |
| **Release** | Workflow listo (`.github/workflows/release.yml`); **no publicado** hasta tag |

### Qué significa

El **código** cubre lo del plan GA (auth/OIDC/TOTP, compose, HA leases, HPA apply, DNS-01, SDKs, Patroni stack, release CI, etc.).  
Lo que **falta para dar por cerrado el producto en git/producción** es operacional: commit/PR, CI verde, tag, smoke E2E estable, validar flags en host real.

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
| S22 | Cerrado en checklist | Cosign/multi-arch workflow presente; tag release pendiente |

---

## 3. Implementado en código (working tree)

| Área | Estado | Evidencia |
|---|---|---|
| Auth + RLS | Listo | migraciones + CI RLS |
| TOTP real + enforce | Listo | `internal/db/totp.go`, mig `000022` |
| OIDC login | Listo | `/auth/oidc/start\|callback` (`api/oidc.go`); guía [`AUTH_OIDC_TOTP.md`](./AUTH_OIDC_TOTP.md) |
| Deploy imagen/git | Listo | localruntime + agent |
| Compose stacks | Listo (WIP local) | `AnalyzeCompose`/`DeployCompose`, mig `000020` |
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
| Release multi-arch+cosign | Workflow listo | `.github/workflows/release.yml` |

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

### Must antes de anunciar / taggear `v1.0.0`

- [ ] **Commit + PR** del working tree GA a `develop` (hoy dirty / no pusheado)
- [ ] **CI verde** en esa PR (`buf breaking`, RLS assert tablas nuevas, go/web/agent)
- [ ] **Migraciones 000020–000022** aplicadas en entornos locales/staging
- [ ] **Smoke E2E**: login → proyecto → deploy (imagen o compose) → `running` + `/healthz` + `/status`
- [ ] **Tag** `v1.0.0` y disparar workflow Release (artifacts + SHA256SUMS + cosign)
- [ ] Validar un nodo agent real (`norix-agent --check` + enroll + workload)

### Should (validación en host, no código faltante)

- [ ] 2 nodos con `NORIX_APPLY_WIREGUARD=1` (`wg show`)
- [ ] Micro-VM en máquina con `/dev/kvm` + artefacts
- [ ] DNS-01 con provider real (Cloudflare/Route53/…)
- [ ] Patroni failover smoke (`deploy/postgresql/patroni`)
- [ ] Backup managed DB a S3-compatible

### No es “código pendiente de sprint”

Features gated por hardware/credenciales **ya están implementadas**; solo requieren activación y prueba en el entorno destino.

---

## 5. Smoke local

```bash
curl -s http://127.0.0.1:8080/healthz   # ok
curl -s http://127.0.0.1:8080/status    # JSON components
```

Quick Start: README raíz.
