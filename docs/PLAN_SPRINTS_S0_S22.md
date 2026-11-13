# Plan de sprints S0–S22 (canónico)

**Alcance:** de cero a **GA 1.0** (~23 sprints, S0…S22, ~2 semanas c/u).  
**Fuente operativa de estado:** `docs/ESTADO_Y_PLAN_V1.md` (marca qué sprint está abierto).  
**Metodología:** cada sprint termina instalable + demo; no abrir el siguiente si falla el DoD.

Equipo de referencia del plan original: 4 personas. Con 1–2, el **orden** se mantiene y el calendario se alarga.

---

## Hitos

| Hito | Sprint | Contenido |
|---|---|---|
| Alpha interna v0.1 | **S6** | Un nodo: git → app con SSL/dominio |
| Beta privada v0.5 | **S10** | Multi-nodo, red, observabilidad, DB + backup |
| Beta sandboxes v0.8 | **S14** | Micro-VMs, SDK/CLI, PR previews |
| RC v0.95 | **S21** | Wasm/HA/cuotas endurecidos, docs, beta pública |
| **GA 1.0** | **S22** | Producción, binarios firmados, soporte |

---

## Fase 0 — Fundaciones

### S0 · Monorepo, CI y validación técnica
- Monorepo, Makefile, buf, CI, spikes (containerd/KVM/Caddy), ADRs iniciales.
- **DoD:** CI verde; riesgos técnicos conocidos documentados.

---

## Fase 1 — MVP un nodo · Alpha (S1–S6)

### S1 · Esqueleto del control plane
- ConnectRPC, Postgres, auth/sesiones, RBAC básico, projects/workloads, auditoría.

### S2 · Dashboard base (React)
- Vite + React, login/proyectos, `go:embed`, smoke e2e.

### S3 · Agente Rust, PKI y enrolamiento
- Join token, stream/heartbeat, capacidades, nodo en UI, `install-agent.sh`.

### S4 · Reconciliación y runtime de contenedores
- Desired/observed, Docker/containerd, deploy imagen, logs.

### S5 · Git, builds y registry
- Webhook/git, BuildKit, zot, logs de build.

### S6 · Edge, SSL, dominios y secretos — **Alpha v0.1**
- Caddy, dominios, ACME, secretos cifrados, health/rollback, install firmado.
- **Hito:** VPS limpio → git/image → HTTPS.

---

## Fase 2 — Multi-nodo y operación · Beta privada (S7–S10)

### S7 · Scheduler multi-nodo
- Capacidades, spread/binpack, réplicas multi-nodo, cordon/drain, reprogramación &lt; 60s.

### S8 · Red entre nodos
- WireGuard mesh, DNS interno, aislamiento por proyecto, egress, edge multi-nodo / DNS-01.

### S9 · Observabilidad y dashboard en vivo
- Métricas/gráficas, logs ingest+búsqueda, Prometheus/OTel, alertas, auditoría viva.

### S10 · Bases de datos 1-click y backups — **Beta privada v0.5**
- Catálogo PG/MySQL/Redis/Mongo, backups (S3), restore 1-click, credenciales como secretos.
- **Hito:** 5–10 usuarios beta invitados.

---

## Fase 3 — Micro-VMs y sandboxes (S11–S14)

### S11 · Runtime micro-VM y guest agent
- Cloud Hypervisor (u elegido), rootfs, vsock guest agent, límites.

### S12 · Snapshots, pools y API sandboxes
- Snapshot/restore, pool caliente, Sandbox TTL, API create/exec/files.

### S13 · SDKs, CLI e intérprete de código (IA)
- SDK TS/Python/Go, CLI, code interpreter, egress deny-by-default.

### S14 · Previews por PR y builds aislados — **Beta sandboxes v0.8**
- Preview efímero por PR, builds en micro-VM, DB de prueba desechable.

---

## Fase 4 — Wasm, empresa y HA (S15–S18)

### S15 · WebAssembly de primera clase
- wasm-worker aislado, OCI components, WASI HTTP, límites fuel/memoria.

### S16 · Multi-equipo, cuotas y medición
- Teams, SSO/OIDC/2FA, cuotas, usage export, rate limit por token.

### S17 · Alta disponibilidad del control plane
- Réplicas CP, Postgres HA (Patroni/managed), líder, failover &lt; 60s.

### S18 · Escalado vertical y autoscaling
- Hotplug/cgroups, HPA por CPU/RPS, scale-to-zero Wasm/previews.

---

## Fase 5 — Endurecimiento y lanzamiento (S19–S22)

### S19 · Seguridad y cadena de suministro
- Threat model, fuzz/escape tests, SBOM, firmas, govulncheck/cargo-deny, pentest.

### S20 · Rendimiento, carga y caos
- k6, soak 72h, chaos, SLOs, runbooks.

### S21 · Documentación, upgrades y beta pública — **RC v0.95**
- Docs site, N-1 upgrades, telemetría opt-in, status page.

### S22 · Candidato final y **GA 1.0**
- Bug bash, API freeze, changelog, SECURITY.md, binarios firmados, anuncio.

---

## Reglas

1. El número de sprint **no se renumera**: un adelanto de código (scaffold) no cierra el sprint.
2. Un sprint solo se marca **cerrado** con evidencia en `docs/SPRINT_N_CIERRE.md` (o equivalente) y DoD verde.
3. Trabajar scaffolds de sprints futuros está permitido, pero el **sprint activo** es el primero incompleto de la secuencia.
