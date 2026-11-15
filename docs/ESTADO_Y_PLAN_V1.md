# Norix — Estado vs plan S0–S22

**Fecha:** 2026-11-15  
**Plan canónico:** [`docs/PLAN_SPRINTS_S0_S22.md`](./PLAN_SPRINTS_S0_S22.md)  
**Hito código:** **Beta** (`VERSION` = `1.0.0-beta.2`) — superficie S0–S22 + installer docker/bare, **no GA**  
**Hito release:** tag `v1.0.0-beta.2`; `main` alineado con develop tras promote

---

## 1. Dónde estamos (honesto)

| | |
|---|---|
| **Fase plan** | S0–S22 con checklist de código; anuncio GA **aplazado** a favor de beta pública |
| **Código** | Superficie amplia en `feature/ga-1.0-completion` → PR a `develop` |
| **Remoto** | Sin tag `v*`; `main` aún es stub temprano (buf), no tip de producción |
| **CI** | Históricamente rojo por `buf breaking` vs `main`; baseline corregida a rama base del PR |
| **Release** | Workflow listo (`.github/workflows/release.yml`); no publicado hasta tag beta |

### Por qué Beta 1 y no Alpha / GA

- **Alpha** (plan S6) ya quedó atrás: hay multi-nodo, DBs, sandboxes, HA/HPA, SDKs.
- **GA 1.0** es prematuro: CI no verde de punta a punta, `main` desfasado, features ops gated sin smoke firmado, sin assets firmados en GitHub Releases.
- **Beta 1** (`1.0.0-beta.1`) comunica “track 1.0 usable, API/ops aún en endurecimiento”.

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
| S22 | Código listo / release abierto | Cosign/multi-arch workflow; tag **beta** pendiente (GA después) |

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
| Release multi-arch+cosign | Workflow listo | `.github/workflows/release.yml` |
| License / contrib | Listo | `LICENSE` (Apache-2.0), `CONTRIBUTING.md` |

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

### Must antes de anunciar / taggear `v1.0.0-beta.1`

- [ ] **PR verde** a `develop` (buf breaking vs base, RLS/migrations, go/web/agent)
- [ ] **Migraciones 000020–000022** aplicadas en entornos locales/staging
- [ ] **Smoke E2E**: login → proyecto → deploy (imagen o compose) → `running` + `/healthz` + `/status`
- [ ] **Tag** `v1.0.0-beta.1` y disparar workflow Release
- [ ] Validar un nodo agent real (`norix-agent --check` + enroll + workload)
- [ ] (Should) Smoke **3 nodos** según [`runbooks/BETA1_3NODE_INSTALL.md`](./runbooks/BETA1_3NODE_INSTALL.md)

### Must antes de GA `v1.0.0`

- [ ] `main` fast-forward al tip liberado; buf breaking baseline = tag GA o `main`
- [ ] Smoke E2E estable repetible + al menos un host de producción de referencia
- [ ] Validación host: WG / KVM / DNS-01 / Patroni / S3 según matriz soportada
- [ ] Docs de upgrade probadas N-1 → N

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
