# Plan diario de Norix

Ruta de aprendizaje e implementación de Norix. Está pensada para mejorar simultáneamente Go, Rust, Linux, cloud e ingeniería de sistemas.

## Objetivo

Construir una plataforma PaaS instalable, segura y observable, sin saltar pasos técnicos ni acumular componentes no probados.

## Cómo trabajar cada día

1. Estudiar el concepto antes de editar código.
2. Implementar la versión mínima.
3. Ejecutar pruebas y comandos de verificación.
4. Observar el comportamiento real del sistema.
5. Documentar el resultado y los errores encontrados.
6. Registrar decisiones relevantes como ADR.
7. No comenzar el siguiente sprint si la entrega actual falla.

## Definición de terminado

- Código compilado y probado.
- Contratos protobuf compatibles o cambios aprobados.
- Secretos fuera de Git.
- Comando reproducible de demostración.
- Logs y métricas mínimas.
- Documentación de operación.
- Riesgos registrados.

## Estado actual — actualización verificable

### Completado

- ✅ Contratos protobuf Buf y generación Go/TypeScript inicial.
- ✅ Control plane HTTP/ConnectRPC con shutdown controlado y `/healthz`.
- ✅ Dockerfile Go multi-stage, usuario no-root y `.dockerignore`.
- ✅ Instalador Docker MVP que genera secretos y arranca el stack.
- ✅ PostgreSQL 16 persistente, imagen local disponible y Compose validado.
- ✅ `pgxpool`, timeout de ping, lifecycle y cierre de conexiones.
- ✅ Migraciones embebidas mediante `golang-migrate`; versión 1 aplicada sin estado sucio.
- ✅ Tablas `organizations`, `users` y `sessions`.
- ✅ Row Level Security habilitado y forzado en autenticación.
- ✅ Roles `norix` administrativo y `norix_app` sin privilegios superusuario.
- ✅ Argon2id para passwords, `crypto/rand` para sesiones y SHA-256 del token almacenado.
- ✅ Login RPC persistente e inserción de sesión con expiración de 12 horas.
- ✅ RPCs `GetBootstrapStatus` y `CreateInitialAdmin` definidos.
- ✅ Primer owner protegido por lock transaccional PostgreSQL.
- ✅ Contraseña inicial con mínimo de 12 caracteres.
- ✅ Pruebas Go de API, autenticación y servidor pasando.

### Pendiente o incompleto

- 🔄 Probar desde Docker la creación real del owner y su login.
- 🔄 Devolver la sesión y el usuario al crear el administrador inicial.
- 🔄 Logout, renovación, revocación y limpieza de sesiones expiradas.
- 🔄 Middleware de autenticación Bearer y contexto de usuario/tenant.
- 🔄 Frontend React: primer acceso, creación de administrador y login.
- 🔄 Aplicar RLS a consultas de aplicación usando `norix_app`, no al usuario migrador.
- 🔄 Rate limiting y protección anti enumeración de login/bootstrap.
- 🔄 Validar email, slug, contraseña fuerte y unicidad de organización.
- 🔄 CLI Go, dashboard completo y agente Rust.
- 🔄 Caddy público, dominio y TLS automático.
- 🔄 Backup/restore, systemd, upgrade y rollback.

### Flujo de primer arranque

```text
Instalación
  -> PostgreSQL healthy
  -> migraciones aplicadas
  -> frontend carga GetBootstrapStatus
       -> owner existente: mostrar login
       -> owner inexistente: mostrar creación de administrador
Crear administrador
  -> validar datos
  -> Argon2id de la contraseña
  -> transacción PostgreSQL
  -> advisory lock
  -> crear organización
  -> crear usuario role=owner
  -> crear sesión
  -> mostrar dashboard autenticado
```

### Evidencia obtenida

```text
go test ./...                 PASS
buf lint                     PASS
PostgreSQL 16                 healthy
schema_migrations version 1   dirty=false
norix_app                     non-superuser
organizations/users/sessions  RLS enabled/forced
GET /healthz                  200 ok
```

### Decisiones pendientes con aprendizaje asociado

- Crear un frontend propio en React permitirá entender su conexión con Connect y WebAssembly HTTP.
- La API de bootstrap debe permanecer activa únicamente hasta crear el owner, pero el frontend también debe confirmar el estado desde backend.
- Los tokens JWT futuros deben distinguirse de las sesiones opacas actuales: empezamos con revocación explícita y DB.
- El instalador Docker debe migrar instalaciones existentes sin cambiar contraseñas ni reiniciar volúmenes.

## Fase 0 — instalación y fundaciones

### Día 1: instalación Docker

- Comprobar Linux, arquitectura y Docker Compose v2.
- Crear `~/.norix` con permisos `0700`.
- Crear `.env` con permisos `0600`.
- Generar PostgreSQL password con `openssl rand`.
- Hacer instalación idempotente.

Aprendizajes: Bash seguro, permisos Unix, Docker Compose, secrets locales.

Verificación:

```bash
NORIX_HOME=/tmp/norix-install-test scripts/install.sh
curl -fsS http://127.0.0.1:8080/healthz
docker compose ps
```

### Día 2: imagen Go

- Completar Dockerfile multi-stage.
- Construir binario estático con CGO desactivado.
- Ejecutar con usuario no privilegiado.
- Añadir `.dockerignore`.
- Configurar `HTTP_ADDR`.

Aprendizajes Go: build flags, módulos, enlace estático y ejecución sin root.
Aprendizajes cloud: capas Docker, caché, superficie mínima de ataque.

### Día 3: lifecycle de servicios

- Healthcheck HTTP real.
- Iniciar control plane tras PostgreSQL healthy.
- Implementar shutdown controlado.
- Revisar restart policy y logs.

Aprendizajes sistemas: procesos, señales, dependencias de arranque, readiness y liveness.

### Día 4: migraciones

- Crear migraciones numeradas.
- Ejecutar migraciones en transacción.
- Bloquear migradores concurrentes.
- Realizar backup antes de subir versión.
- Probar migración hacia arriba y hacia abajo.

Aprendizajes PostgreSQL: DDL transaccional, locks, índices, constraints.
Aprendizajes Go: context, timeouts, pool de conexiones y wrapping de errores.

### Día 5: PostgreSQL desde Go

- Añadir `pgxpool`.
- Leer `DATABASE_URL` al iniciar.
- Validar conexiones y configurar timeout.
- Ejecutar consulta mínima al arrancar.
- Cerrar pool en shutdown.

Código objetivo:

```go
type Database interface {
    Ping(context.Context) error
    Close()
}
```

Aprendizajes Go: interfaces, dependency injection, resources y lifecycle.

### Día 6: RLS real

- Crear `organizations`, `users` y `sessions`.
- Crear rol migrator y rol app separados.
- Configurar tenant dentro de cada transacción.
- Probar aislamiento entre dos organizaciones.
- Asegurar que el cliente no pueda elegir otro tenant sin pertenencia.

SQL conceptual:

```sql
SELECT set_config('app.current_tenant_id', $1, true);
```

### Día 7: owner inicial

- Crear comando interactivo de bootstrap.
- Pedir organización, nombre, email y contraseña.
- Verificar reglas de nombre y email.
- Hashear contraseña con Argon2id.
- Crear usuario owner.
- Impedir múltiples bootstraps no autorizados.

Aprendizajes: CLI, secretos en terminal, validación y operaciones una sola vez.

### Días 8-10: operación y instalación pública

- Implementar `norix status`, `start`, `stop`, `restart` y `logs`.
- Crear unit systemd opcional.
- Crear backup y restore PostgreSQL.
- Preparar instalador remoto HTTPS.
- Probar desde una máquina VPS limpia.
- Publicar versión y checksum firmado.

Instalación objetivo:

```bash
curl -fsSL https://norix.dev/install.sh | bash
```

## Fase 1 — S2-S6 MVP de un nodo

### S2 Dashboard

- React 19, Vite, TanStack Router/Query.
- Login real y listado de proyectos.
- SPA embebida mediante `go:embed`.
- Pruebas Playwright.

### S3 Agente Rust

- Workspace Cargo y `rust-toolchain.toml`.
- Tokio, tonic y Protobuf.
- mTLS con certificados de corta duración.
- Heartbeat y reconexión con backoff.
- Binario musl estático.

Aprendizajes Rust: ownership, lifetimes, async, channels, cancellation, `Result` y errores tipados.

### S4 Contenedores

- containerd por socket Unix.
- Namespaces, cgroups y capabilities.
- Reconciliador: desired state vs observed state.
- Estados Pending, Starting, Running, Stopped y Failed.

Aprendizajes sistemas: procesos, namespaces, control groups, filesystem y redes Linux.

### S5 Builds

- BuildKit.
- Cloud Native Buildpacks opcional.
- Registry OCI Zot.
- Cache e imágenes reproducibles.

### S6 Edge

- Caddy dinámica y TLS.
- Dominios y secretos.
- Probar ACME con Pebble.

Criterio Alpha: Git push produce aplicación ejecutándose con HTTPS.

## Fase 2 — S7-S10 multi-nodo

### S7 Scheduler

- Recursos CPU, RAM y nodos disponibles.
- Bin packing y restricciones.
- Reconciliación y recuperación tras desconexión.

### S8 Red

- WireGuard entre nodos.
- TAP/veth y nftables.
- Aislamiento de egreso y MTU.

### S9 Observabilidad

- Logs estructurados.
- Streaming por Connect/gRPC.
- Métricas y seguimiento de latencia.

### S10 Bases de datos

- Provisioning de PostgreSQL por workload.
- Backups y restore.
- Beta privada.

## Fase 3 — S11-S14 micro-VMs y sandboxes

1. Cloud Hypervisor frente a Firecracker.
2. Kernel minimal y rootfs reproducible.
3. Guest agent sobre vsock.
4. Snapshots y pools de VM.
5. API sandbox.
6. SDK Go, TypeScript y Python.
7. CLI con Cobra.
8. Preview por pull request.

Aprendizajes Rust: virtio, vsock, memoria, procesos y dispositivos virtuales.
Aprendizajes sistemas: KVM, hardware virtualization, TAP, virtio y boot Linux.

## Fase 4 — S15-S18 Wasm, empresa y HA

- Wasmtime/WASI con Landlock/seccomp.
- Teams, quotas y medición.
- PostgreSQL HA con PgBouncer/Patroni si corresponden.
- Reconexión de streams de agentes.
- Hot scaling y scale to zero.

## Fase 5 — S19-S22 lanzamiento

- Modelado de amenazas y pentest.
- Firma, SBOM y reproducibilidad.
- Fuzzing Go/Rust.
- Pruebas de carga y caos.
- SLOs, runbooks y documentación.
- Beta y GA 1.0.

## Plan diario Rust del agente

### Semana 1

1. Crear workspace y toolchain fijado.
2. Parsear configuración.
3. Inicializar logging Tokio.
4. Conectar cliente tonic.
5. Implementar mTLS.
6. Enviar heartbeat.
7. Reconectar con backoff.
8. Añadir tracing.
9. Construir para musl.
10. Integrar con CI.

Conceptos obligatorios: ownership, borrowing, lifetimes, Send/Sync, Arc, channels, async/await, cancellation, errors y protobuf.

## Plan diario Linux

1. Procesos, PID y namespaces.
2. Cgroups CPU/memory/PIDs.
3. Mount namespaces y filesystem.
4. Capabilities y seccomp.
5. Unix sockets.
6. TCP, UDP y señales.
7. Network namespaces y veth.
8. nftables.
9. Journald y logs.
10. systemd de producción.

No ejecutar namespaces, firewall o capacidades sobre una máquina productiva sin una VM de laboratorio.

## ADRs obligatorios

Crear uno por decisión relevante:

- Go como control plane y Rust como agente.
- ConnectRPC.
- PostgreSQL como persistencia principal.
- RLS para multi-tenancy.
- Argon2id.
- containerd.
- Cloud Hypervisor.
- WireGuard.
- Wasmtime.
- Caddy/ACME.

Formato:

```markdown
# ADR-NNN
Estado: propuesta
Contexto:
Decisión:
Alternativas:
Consecuencias:
Criterios de revisión:
```

## Registro diario recomendado

Crear `docs/progress/Sprint-Dia.md` con:

- Qué se implementó.
- Qué se intentó.
- Qué falló.
- Cómo se verificó.
- Qué aprendiste de Go.
- Qué aprendiste de Rust.
- Qué aprendiste de Linux/cloud.
- Riesgos y próximo día.

## Comandos de referencia

```bash
# Go
cd control-plane
go test ./...
CGO_ENABLED=0 go build ./cmd/server

# Protos
cd ..
buf lint
buf generate

# Docker
docker compose -f deploy/norix/docker-compose.yml config
curl -fsS http://127.0.0.1:8080/healthz

git diff --check
git status --short

# Rust, después de crear agent/
cd agent
cargo fmt --check
cargo clippy --all-targets --all-features -- -D warnings
cargo test
```
