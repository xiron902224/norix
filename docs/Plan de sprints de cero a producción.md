# Plan de sprints de cero a producción

Documento convertido automáticamente desde DOCX.

<!-- Fuente: Plan de sprints de cero a producción.docx -->

Plan de sprints: de cero a producción

Hoja de ruta completa de la plataforma PaaS (control plane Go, agente Rust, micro-VMs, Wasm, dashboard React). Documento compañero de la arquitectura final v1.0.

1. Supuestos y metodología

Sprints de 2 semanas, 23 sprints (S0 a S22), aproximadamente 46 semanas (~11 meses).

Equipo de referencia: 4 personas (2 backend Go/Rust de sistemas, 1 full-stack con React/TypeScript, 1 DevOps/seguridad a tiempo parcial). Con un equipo menor, cada fase se alarga proporcionalmente; el orden no cambia. Las estimaciones son una referencia y se ajustan con la velocidad real tras S3.

Cadencia: planificación el primer día, demo y retrospectiva el último. Cada sprint termina con una versión instalable (nightly) y demo funcional.

Prioridad estratégica: la API de sandboxes sobre micro-VMs es la cuña de mercado, por eso llega en la Fase 3, antes que Wasm y las funciones de empresa.

Definición de Hecho (aplica a todo sprint)

Código revisado por al menos otra persona y fusionado con CI en verde (lint, pruebas unitarias, buf breaking).

Pruebas automatizadas del comportamiento nuevo (unitarias y, si toca sistema, e2e).

Documentación actualizada (ADR si hubo decisión, guía de usuario si hay feature visible).

Métricas y logs para lo nuevo; sin regresión de los presupuestos de memoria y arranque.

Instalable de cero con el script en un VPS limpio y demo realizada.

Revisión de seguridad ligera (secretos, permisos, entradas no confiables).

Hitos y versiones

Hito

Sprint

Contenido

Alpha interna (v0.1)

S6

Un nodo: git push → app con SSL y dominio

Beta privada (v0.5)

S10

Multi-nodo, red, observabilidad, bases de datos con backup

Beta de sandboxes (v0.8)

S14

Micro-VMs, SDK, previews por PR, builds aislados

Release candidate (v0.95)

S21

Wasm, HA, cuotas, seguridad auditada, documentación

GA 1.0

S22

Producción con soporte y SLOs publicados

2. Fase 0 — Fundaciones (S0)

## S0 · Monorepo, CI y validación técnica (semanas 1-2)

Objetivo: dejar listo el suelo del proyecto y eliminar los mayores riesgos técnicos antes de construir.

Entregables

Monorepo con la estructura del documento de arquitectura, Makefile, buf y generación de código Go, Rust y TypeScript.

CI en GitHub Actions: lint, pruebas, compilación multi-arch (x86_64 y aarch64), cachés.

Spikes de riesgo: (a) arrancar una micro-VM con Cloud Hypervisor y Firecracker y medir arranque y memoria; (b) Wasmtime en worker aislado con pooling allocator; (c) comprobar KVM en runners de CI y en 2-3 proveedores de VPS/bare-metal.

ADRs iniciales (los 10 de la arquitectura) y borrador del modelo de amenazas.

Decisión documentada: Cloud Hypervisor vs Firecracker para la v1 (con datos del spike).

Criterio de aceptación: make build y make proto funcionan desde un clon limpio; informe de los spikes aprobado.

## Retos de ingeniería (arquitectura §18):

Toolchain: buf lint y buf breaking en CI desde el primer commit; make proto seguido de git diff --exit-code para detectar stubs desactualizados; versiones fijadas (rust-toolchain.toml, directiva toolchain de Go); CI por componente con filtros por ruta, cachés y compilación cruzada musl (x86_64 y aarch64); CODEOWNERS y dos revisores obligatorios en proto/.

KVM: prototipo del crate capabilities con los niveles 0 a 3; prueba de nivel 2 (micro-VM mínima con vsock, red y snapshot) en 2-3 proveedores, que inaugura la matriz de proveedores verificados.

Caddy: spike de ACME contra Pebble y de la API de administración de Caddy (aplicar configuración completa, validar y revertir).

Criterio adicional: un PR típico termina el CI en menos de 10 minutos y el informe incluye la matriz inicial de proveedores.

## Documentación recomendada a dominar:

Buf & Protocol Buffers: Buf Documentation (Buf CLI & Breaking) y Protocol Buffers Documentation.

KVM & Micro-VMs: Linux Kernel KVM API Documentation, Cloud Hypervisor Documentation y Firecracker Getting Started Guide.

Wasmtime & WebAssembly: Wasmtime Guide & Docs y MDN WebAssembly (Español).

Caddy & ACME/Pebble: Caddy API Guide y Pebble ACME Test Server.

Plan diario de trabajo (S0)

Día 1 · Configuración del Monorepo, Buf y Makefile

Documentación: Buf Configuration (buf.yaml) y Buf Code Generation.

Detalle: Inicializar el repositorio Git estructurando los directorios base (proto/, control-plane/, agent/). Configurar buf.yaml con linters estrictos y detección de breaking changes contra la rama principal. Implementar un Makefile que ejecute la generación de código mediante buf generate y compruebe diffs limpios.

## Ejemplo / Solución: En Makefile:

proto-generate:buf lintbuf generategit diff --exit-code proto/

Día 2 · CI Matrix en GitHub Actions y Compilación Cruzada

Documentación: GitHub Actions Matrix Workflow y Rustup Cross-compilation.

Detalle: Definir .github/workflows/ci.yml con matriz para arquitecturas x86_64 y aarch64. Configurar caché de Go (go build) y sccache / cargo-chef para Rust, garantizando tiempos de pipeline inferiores a 10 minutos.

## Ejemplo / Solución: En ci.yml:

jobs:  build:    strategy:      matrix:        target: [x86_64-unknown-linux-musl, aarch64-unknown-linux-musl]    steps:      - uses: actions/checkout@v4      - run: cargo build --target ${{ matrix.target }} --release

Día 3 · Spike Cloud Hypervisor: Telemetría de Arranque y Memoria

Documentación: Cloud Hypervisor API Reference.

Detalle: Instalar Cloud Hypervisor v38+, orquestar el arranque de un kernel custom con initramfs mínimo y registrar la latencia de boot y consumo de memoria del proceso host usando sockets UNIX.

## Ejemplo / Solución: Comando de ejecución directa vía API socket:

cloud-hypervisor --api-socket /tmp/ch.sock &curl --unix-socket /tmp/ch.sock -X PUT "http://localhost/api/v1/vm.create" \  -H "Content-Type: application/json" -d '{"kernel":{"path":"vmlinux"},"memory":{"size":536870912}}'curl --unix-socket /tmp/ch.sock -X PUT "http://localhost/api/v1/vm.boot"

Día 4 · Spike Firecracker: Benchmark Comparativo de Micro-VMs

Documentación: Firecracker MicroVM Setup.

Detalle: Ejecutar pruebas equivalentes con Firecracker API, contrastando el soporte de virtio-pmem, vsock y la velocidad de cold boot (< 150 ms) versus las capacidades virtio de Cloud Hypervisor.

## Ejemplo / Solución: Configuración del socket Firecracker en Bash:

curl --unix-socket /tmp/firecracker.socket -X PUT "http://localhost/boot-source" \  -d '{"kernel_image_path":"vmlinux","boot_args":"console=ttyS0 reboot=k panic=1 pci=off"}'

Día 5 · Spike Wasmtime: Worker Aislado y Pooling Allocator

Documentación: Wasmtime Pooling Allocation Config.

Detalle: Construir prototipo Rust embebiendo Wasmtime con configuración de asignador por grupos pre-reservados (pooling allocator) y límites de fuel para ejecución aislada de funciones ligeras.

## Ejemplo / Solución: Inicialización en Rust:

let mut config = wasmtime::Config::new();let mut pool = wasmtime::PoolingAllocationConfig::default();pool.total_core_instances(1000).max_memory_size(64 * 1024 * 1024);config.allocation_strategy(wasmtime::InstanceAllocationStrategy::Pooling(pool));config.consume_fuel(true);

Día 6 · Verificación de KVM y Entornos Anidados en Proveedores

Documentación: Linux KVM API (Kernel Docs).

Detalle: Ejecutar scripts de validación de KVM (/dev/kvm, permisos rw y soporte de virtualización anidada) en runners de GitHub Actions y VPS de Hetzner, OVH y DigitalOcean.

## Ejemplo / Solución: Comprobación en shell:

kvm-ok || (test -r /dev/kvm && test -w /dev/kvm)egrep -c '(vmx|svm)' /proc/cpuinfo

Día 7 · Implementación del Crate Capabilities (Niveles KVM 0 a 3)

Documentación: Rust Nix Crate File Status.

Detalle: Crear en Rust el crate modular capabilities clasificando el soporte de virtualización: Nivel 0 (sin KVM), Nivel 1 (disponible pero sin permisos), Nivel 2 (KVM operativo), Nivel 3 (KVM con arranque verificado de micro-VM efímera).

## Ejemplo / Solución: En Rust:

pub fn detect_kvm_level() -> u8 {    match std::fs::OpenOptions::new().read(true).write(true).open("/dev/kvm") {        Ok(_) => 2,        Err(e) if e.kind() == std::io::ErrorKind::PermissionDenied => 1,        Err(_) => 0,    }}

Día 8 · Spike Caddy: API Dinámica y Validación con Pebble ACME

Documentación: Caddy Admin API y Let's Encrypt Pebble Server.

Detalle: Levantar Caddy y Pebble en Docker Compose. Validar la aplicación de configuraciones de enrutamiento mediante POST /load y comprobar la reversión ante configs erróneas sin caída de servicio.

## Ejemplo / Solución: Mutación JSON vía curl:

curl -X POST "http://localhost:2019/load" \  -H "Content-Type: application/json" \  -d '{"apps":{"http":{"servers":{"srv0":{"listen":[":80"],"routes":[{"handle":[{"handler":"static_response","body":"ok"}]}]}}}}}'

Día 9 · Redacción de ADRs y Modelo de Amenazas (STRIDE)

Documentación: Architecture Decision Records Pattern.

Detalle: Redactar formalmente del ADR-001 al ADR-010 en docs/adr/, documentando la selección de micro-VMs vs contenedores puros, división Go/Rust y modelo de límites defensivos.

Ejemplo / Solución: Estructura del ADR en Markdown con campos Status: Accepted, Context, Decision y Consequences.

Día 10 · Aprobación de Informe de Spikes y Cierre de S0

Documentación: GitHub PR Review Guidelines.

Detalle: Consolidar las métricas de arranque y memoria obtenidas. Aprobar la selección de Cloud Hypervisor para la v1 y certificar que la pipeline de CI finalice en verde de punta a punta.

## Ejemplo / Solución: Ejecución de verificación completa previa a merge:

make build && make test && make proto && git diff --exit-code

3. Fase 1 — MVP de un nodo (S1-S6) · Alpha interna

## S1 · Esqueleto del control plane

Entregables: servidor Go con ConnectRPC; capa de datos con sqlc y migraciones para SQLite y Postgres; usuarios, sesiones, tokens de API y RBAC básico (propietario, administrador, miembro); modelo de datos de proyectos y workloads; registro de auditoría; configuración y logs estructurados.

Aceptación: se crea usuario y proyecto por API; pruebas de integración contra SQLite y Postgres; binario de control plane < 150 MB de RAM en reposo.

## Documentación recomendada a dominar:

ConnectRPC (Go): ConnectRPC Go Documentation y especificación del protocolo Connect.

sqlc & Postgres/SQLite: sqlc Documentation, PostgreSQL Official Documentation y SQLite Reference.

Go Concurrencia & Arquitectura: Effective Go y Documentación Go en Español.

Plan diario de trabajo (S1)

Día 1 · Servidor base ConnectRPC y middleware estructurado

Documentación oficial: ConnectRPC Go Getting Started y Go slog Package.

Descripción de tareas: Configurar servidor HTTP/2 con soporte ConnectRPC, implementando interceptores globales para logging estructurado en JSON con slog y correlación de request ID.

## Ejemplo / Solución: Interceptor básico en Go:

func NewLoggingInterceptor() connect.UnaryInterceptorFunc { return func(next connect.UnaryFunc) connect.UnaryFunc { return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) { slog.Info("call", "proc", req.Spec().Procedure); return next(ctx, req) } } }

Día 2 · Esquema de base de datos con sqlc y migraciones

Documentación oficial: sqlc Documentation y golang-migrate GitHub.

Descripción de tareas: Redactar esquemas DDL SQL iniciales y consultas fuertemente tipadas en Go mediante sqlc con soporte dual Postgres/SQLite.

## Ejemplo / Solución: En sqlc.yaml:

version: "2"

## sql:

- schema: "internal/db/migrations"

queries: "internal/db/queries"

## gen:

## go:

package: "db"

out: "internal/db"

Día 3 · Modelo de usuarios, sesiones y hashing seguro

Documentación oficial: Argon2 Go Documentation.

Descripción de tareas: Implementar persistencia de usuarios y generación segura de sesiones utilizando derivación de claves con Argon2id.

## Ejemplo / Solución: Hash de contraseña en Go:

hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)

Día 4 · Autenticación de servicios mediante API Tokens

Documentación oficial: OWASP Key Management Cheat Sheet.

Descripción de tareas: Crear generador de tokens de API con prefijo identificador (ej. paas_live_...) y almacenamiento hashed con SHA-256 en base de datos.

## Ejemplo / Solución: Verificación de hash SHA-256 en Go:

tokenHash := sha256.Sum256([]byte(rawToken))

Día 5 · RBAC básico e interceptores de autorización

Documentación oficial: ConnectRPC Interceptors Guide.

Descripción de tareas: Diseñar interceptor de autorización validando roles (propietario, admin, miembro) y alcance de permisos sobre las entidades consultadas.

## Ejemplo / Solución: Validación de contexto en interceptor ConnectRPC:

if !hasRole(ctx, RoleAdmin) { return nil, connect.NewError(connect.CodePermissionDenied, errors.New("permiso denegado")) }

Día 6 · CRUD de Proyectos y Workloads en ConnectRPC

Documentación oficial: Buf Code Generation Overview.

Descripción de tareas: Implementar servicios gRPC/Connect para creación, consulta, modificación y eliminación de proyectos y workloads.

## Ejemplo / Solución: Handler de servicio en Go:

func (s *ProjectServer) CreateProject(ctx context.Context, req *connect.Request[v1.CreateProjectRequest]) (*connect.Response[v1.CreateProjectResponse], error) { ... }

Día 7 · Registro de auditoría transaccional

Documentación oficial: PostgreSQL Transactions Tutorial.

Descripción de tareas: Guardar logs inmutables de auditoría en la misma transacción SQL que ejecuta cambios en los recursos.

## Ejemplo / Solución: Registro en bloque de transacción:

tx.ExecContext(ctx, "INSERT INTO audit_logs (user_id, action, target) VALUES ($1, $2, $3)", uid, action, target)

Día 8 · Pruebas de integración automatizadas SQLite / Postgres

Documentación oficial: Go Testing Package y Testcontainers Postgres for Go.

Descripción de tareas: Configurar batería de tests de integración comprobando coherencia idéntica de la capa de datos entre SQLite en memoria y PostgreSQL en contenedor.

## Ejemplo / Solución: Test unitario dual:

func TestDatabaseCompat(t *testing.T) { db := setupTestDB(t); runProjectTests(t, db) }

Día 9 · Profiling pprof y verificación de memoria en reposo

Documentación oficial: Profiling Go Programs (Go Blog).

Descripción de tareas: Analizar el uso de heap y corrutinas del control plane con pprof, validando consumo inferior a 150 MB de memoria en reposo.

## Ejemplo / Solución: Servidor pprof:

go func() { log.Println(http.ListenAndServe("localhost:6060", nil)) }()

Día 10 · Demo interna y cierre de DoD de S1

Documentación oficial: ConnectRPC Documentation.

Descripción de tareas: Ejecutar flujo completo de creación de usuarios y workloads vía curl/CLI contra ConnectRPC y validar checklist de DoD del sprint.

## Ejemplo / Solución: Llamada curl de verificación:

curl -X POST http://localhost:8080/v1.ProjectService/CreateProject -H "Content-Type: application/json" -d '{"name":"demo"}'

## S2 · Dashboard base (React)

Entregables: SPA con Vite + React 19 + TypeScript, TanStack Router y Query, Tailwind y shadcn/ui; cliente Connect generado; login, listado y creación de proyectos; embebida en el binario con go:embed y servida con caché y compresión; Playwright con un flujo de humo.

Aceptación: el dashboard carga desde el binario único, sin proceso Node; prueba e2e de login y creación de proyecto.

## Documentación recomendada a dominar:

React 19 & Vite: React Official Docs / Documentación de React en Español y Vite Guide.

TanStack (Router & Query): TanStack Router Docs y TanStack Query Docs.

Tailwind CSS & shadcn/ui: Tailwind CSS Documentation y shadcn/ui Documentation.

Plan diario de trabajo (S2)

Día 1 · Configuración inicial Vite + React 19 + shadcn/ui

Documentación oficial: Vite Guide y shadcn/ui Vite Installation.

Descripción de tareas: Inicializar el proyecto web frontend con Vite, TypeScript, Tailwind CSS y componentes primitivos de shadcn/ui.

## Ejemplo / Solución: Comando de inicialización:

npm create vite@latest web -- --template react-ts && cd web && npx shadcn@latest init

Día 2 · Generación de clientes TypeScript con Connect-ES

Documentación oficial: Connect-ES Getting Started.

Descripción de tareas: Configurar plugin buf.gen.yaml para generar tipos y clientes fuertemente tipados de ConnectRPC en TypeScript.

## Ejemplo / Solución: En buf.gen.yaml:

## plugins:

- remote: buf.build/connectrpc/es:v1.4.0

out: web/src/gen

opt: target=ts

Día 3 · Enrutamiento y consultas con TanStack Router & Query

Documentación oficial: TanStack Router Docs y TanStack Query Docs.

Descripción de tareas: Integrar el enrutador tipado TanStack Router y el gestor de estado asíncrono TanStack Query para fetching de endpoints Connect.

## Ejemplo / Solución: Configuración de transporte en React:

const transport = createConnectTransport({ baseUrl: "/api" });

Día 4 · Pantalla de login y gestión de sesión

Documentación oficial: React Form Actions Docs.

Descripción de tareas: Diseñar la vista de inicio de sesión con validación de formularios y almacenamiento seguro de cookies de sesión HttpOnly.

## Ejemplo / Solución: Hook de autenticación:

const { mutate: login } = useMutation({ mutationFn: (creds) => authClient.login(creds) });

Día 5 · Listado y creación interactiva de proyectos

Documentación oficial: shadcn/ui Dialog Component.

Descripción de tareas: Implementar tabla dinámica de proyectos y cuadro modal de creación con refresco automático de la caché de TanStack Query.

## Ejemplo / Solución: Invalidación de consultas:

queryClient.invalidateQueries({ queryKey: ['projects'] });

Día 6 · Vista de detalle de proyecto y estado de workloads

Documentación oficial: shadcn/ui Badge Component.

Descripción de tareas: Desarrollar interfaz con badges de estado (running, stopped, degraded) y métricas resumidas por workload.

## Ejemplo / Solución: Componente visual de estado:

<Badge variant={workload.status === 'RUNNING' ? 'default' : 'destructive'}>{workload.status}</Badge>

Día 7 · Embebido de SPA en binario Go mediante go:embed

Documentación oficial: Go embed Package.

Descripción de tareas: Empaquetar la distribución compilada web/dist directamente dentro del ejecutable del plano de control en Go.

## Ejemplo / Solución: Directiva Go embed:

//go:embed dist/*

var webFS embed.FS

Día 8 · Compresión gzip y cabeceras de caché HTTP

Documentación oficial: MDN Cache-Control (Español).

Descripción de tareas: Configurar servidor Go para servir los assets estáticos con compresión automática y headers inmutables para assets cacheados.

## Ejemplo / Solución: Middleware de caché:

w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

Día 9 · Pruebas e2e con Playwright

Documentación oficial: Playwright Getting Started.

Descripción de tareas: Implementar flujo de humo automatizado verificando login, renderizado del dashboard y creación de proyecto nuevo.

## Ejemplo / Solución: Test de Playwright:

test('login flow', async ({ page }) => { await page.goto('/login'); await page.fill('#user', 'admin'); await page.click('button[type=submit]'); });

Día 10 · Verificación de binario autosuficiente y demo

Documentación oficial: Documentación oficial Go.

Descripción de tareas: Compilar binario único final sin dependencias de Node.js en el host y ejecutar prueba en limpio.

## Ejemplo / Solución: Comando de verificación:

go build -o server ./cmd/server && ./server --port=8080

## S3 · Agente Rust, PKI y enrolamiento

Entregables: agente con Tokio; CA interna en el control plane; token de unión de un solo uso → CSR → certificado de 24 h con rotación; stream gRPC saliente con mTLS y reconexión con backoff; detección de capacidades (KVM, cgroups v2, recursos); latidos y estado del nodo visibles en el dashboard; install-agent.sh v1 (con sh -s -- --token).

Aceptación: un nodo se enrola con un comando, aparece en el dashboard y se recupera tras reiniciar red o servicio; agente < 50 MB en reposo.

## Retos de ingeniería (arquitectura §18):

KVM: crate capabilities completo con los niveles 0 a 3, incluida la micro-VM de prueba; el agente reporta microvm: {available, reason, nested, boot_ms} y lo revalida periódicamente; el instalador muestra el resultado con una explicación clara y enrola el nodo igualmente.

Toolchain: suite de conformidad Go↔Rust en tests/e2e/ (agente simulado en Go y control plane simulado en Rust ejecutando los mismos casos contra el contrato).

Criterio adicional: un nodo sin KVM se enrola, aparece en el dashboard con capacidades de contenedores y Wasm y con la razón por la que no ofrece micro-VMs; la suite de conformidad corre en cada PR.

## Documentación recomendada a dominar:

Rust Asíncrono con Tokio: Tokio Tutorial & Guide y The Rust Programming Language (El Libro de Rust).

Tonic & gRPC en Rust: Tonic Documentation y gRPC Official Core Concepts.

PKI, X.509 y mTLS: Rustls Documentation y Guía de mTLS en Español (Cloudflare Learning).

Plan diario de trabajo (S3)

D1: Estructura del crate agent en Rust con runtime Tokio y trazabilidad tracing. D2: Implementación de la CA interna X.509 en el control plane para emisión de mTLS. D3: Flujo de enrolamiento con tokens de unión de un solo uso y generación de CSR en el agente. D4: Rotación automática de certificados de 24 horas y renegociación transparente de TLS. D5: Stream bidireccional gRPC saliente desde el agente con backoff exponencial. D6: Detección de hardware y kernel: KVM (niveles 0-3), cgroups v2, CPU y RAM. D7: Reporte de capacidades y latidos del nodo reflejados en tiempo real en el dashboard. D8: Elaboración del script install-agent.sh para aprovisionamiento con un comando. D9: Suite de conformidad de contratos Go↔Rust en tests/e2e/ y verificación de memoria < 50 MB. D10: Demo de enrolamiento en un VPS real con recuperación ante reinicio del servicio.

## S4 · Reconciliación y runtime de contenedores

Entregables: workload.proto y agent.proto declarativos y versionados; reconciliador (servidor y agente); runtime-containerd con crear, iniciar, detener, exec, logs y stats; despliegue de una imagen pública desde el dashboard; reintentos idempotentes; captura de logs en vivo (streaming) y terminal web con xterm.js.

Aceptación: desplegar, actualizar y eliminar una imagen; matar el agente a mitad de despliegue y verificar que reconcilia al volver.

## Retos de ingeniería (arquitectura §18):

Reglas de Tokio en el agente: timeout y cancelación en todo await externo, canales acotados, llamadas bloqueantes en spawn_blocking, apagado ordenado con CancellationToken y JoinSet, tracing en los límites y tokio-console en desarrollo; todo unsafe aislado en crates pequeños con revisión obligatoria.

Sockets Unix: cliente tipado con reintentos para la API de los runtimes, probado contra un servidor falso.

Punto de revisión del toolchain: al cerrar este sprint se mide la fricción de Go + Rust (tiempo de CI, contratos rotos, tiempo de incorporación). Si es desproporcionada se reevalúa, mediante un ADR, la opción de un agente en Go con Rust solo para wasm-worker y guest-agent.

Criterio adicional: pruebas de cancelación y de apagado ordenado en verde; ADR de revisión del toolchain publicado.

## Documentación recomendada a dominar:

containerd & OCI Runtime: containerd Documentation y OCI Runtime Specification.

Patrones de Reconciliación: Controladores y Reconciliación (Kube Docs en Español).

Tracing y Diagnóstico en Tokio: Tokio Tracing & Diagnostics y tokio-console Reference.

Plan diario de trabajo (S4)

D1: Definición y versionado de contratos declarativos workload.proto y agent.proto. D2: Crate runtime-containerd en Rust para comunicación con containerd vía Unix domain socket. D3: Máquina de estados del reconciliador en el agente para asegurar convergencia del workload. D4: Operaciones del ciclo de vida del contenedor: pull de imagen, create, start y graceful stop. D5: Muestreo y streaming de estadísticas de recursos de cgroups v2 hacia el control plane. D6: Streaming de logs stdout/stderr en tiempo real a través de gRPC. D7: Implementación de terminal web interactiva con exec en containerd y xterm.js en React. D8: Manejo de resiliencia: apagado ordenado, CancellationToken y cancelación en llamadas await. D9: Pruebas de caos: matar el agente en pleno despliegue y verificar reconciliación al reiniciar. D10: Publicación del ADR de revisión de toolchain Go/Rust y demo de despliegue de imagen pública.

## S5 · Git, builds y registry

Entregables: GitHub App (y webhooks genéricos) con GitLab y Bitbucket en modo básico; clonado y detección de stack; builder con BuildKit y Buildpacks (en contenedor por ahora); registry zot integrado; cola de builds con logs en vivo y caché de capas.

Aceptación: git push de un repositorio Node, Python y Go de ejemplo produce imagen y despliegue automáticos.

## Documentación recomendada a dominar:

BuildKit: BuildKit Architecture and Guide y Docker BuildKit Reference.

Cloud Native Buildpacks (CNB): Buildpacks Official Documentation & Concepts.

Registro OCI con Zot: Zot Registry Official Docs y especificación OCI Distribution.

Plan diario de trabajo (S5)

D1: Integración con GitHub App y receptor de webhooks genéricos para git push. D2: Servicio de clonado seguro de repositorios con soporte de submódulos y tokens temporales. D3: Detección automática de stacks tecnológicos (Node.js, Go, Python, Dockerfile). D4: Integración de BuildKit con socket aislado para construcción eficiente de imágenes OCI. D5: Soporte de Cloud Native Buildpacks (CNB) para repositorios sin Dockerfile. D6: Despliegue y configuración del registro interno OCI embebido Zot con autenticación por token. D7: Cola de builds con gestión de prioridades y límites de concurrencia. D8: Streaming de logs del build en directo hacia el dashboard mediante WebSockets/Connect. D9: Pruebas de caché de capas OCI para builds incrementales en menos de 30 segundos. D10: Demo end-to-end: git push de repositorios Node, Python y Go con despliegue automático.

## S6 · Edge, SSL, dominios y secretos — Alpha v0.1

Entregables: control de Caddy por API; rutas por dominio; certificados Let's Encrypt con renovación sin caída; variables de entorno y secretos cifrados (age); despliegue con health checks, azul/verde simple y rollback automático; install.sh del control plane firmado (cosign/minisign) y con uninstall/upgrade.

Aceptación: en un VPS limpio, instalación en < 3 min y git push → aplicación en HTTPS con dominio propio. Demo Alpha.

## Retos de ingeniería (arquitectura §18):

Un solo escritor de la configuración de Caddy: el agente genera la configuración completa desde las Route deseadas, la valida antes de aplicarla, la aplica sin interrupción y conserva la última válida para revertir; usa control optimista con ETag si la versión de Caddy lo permite y, si no, serializa los cambios.

Interfaz CertProvider: en un nodo, implementación con ACME nativo de Caddy, preparada para cambiar a emisión centralizada en S8.

Protección de Let's Encrypt: comprobar que el DNS apunta al edge antes de pedir el certificado; servidor de staging en desarrollo y CI; Pebble en las pruebas e2e.

Observabilidad: métricas cert_expiry_seconds y acme_failures_total, y alerta a 14 días del vencimiento.

Criterio adicional: una configuración inválida se rechaza y se mantiene la anterior sin cortes.

## Documentación recomendada a dominar:

Caddy Web Server: Caddy Architecture & Modules y Automatic HTTPS Guide.

ACME & Let's Encrypt: Cómo funciona Let's Encrypt (Español) y RFC 8555 (Automatic Certificate Management Environment).

Criptografía con age & Cosign: age Encryption Tool and Go Library y Sigstore Cosign Documentation.

Plan diario de trabajo (S6)

D1: Cliente de API de administración de Caddy con generación de configuración declarativa. D2: Mecanismo de validación previa y rollback atómico ante errores de configuración en Caddy. D3: Configuración de enrutamiento dinámico por subdominio y host header. D4: Automatización de certificados Let's Encrypt con verificación previa de registros DNS. D5: Almacenamiento seguro de secretos y variables de entorno cifrados en reposo con age. D6: Motor de despliegue azul/verde con health checks HTTP y conmutación de tráfico sin cortes. D7: Rollback automático si la nueva versión falla los health checks iniciales. D8: Elaboración del instalador install.sh firmado con Cosign/minisign y comandos de upgrade. D9: Pruebas de instalación completa en un VPS limpio en menos de 3 minutos. D10: Demostración de Alpha v0.1: git push a app en producción con HTTPS automático.

4. Fase 2 — Multi-nodo y operación (S7-S10) · Beta privada

## S7 · Scheduler multi-nodo

Entregables: scheduler con capacidades, recursos libres, afinidad/anti-afinidad y zonas; estrategia bin packing y spread; réplicas de un workload en varios nodos; drain y cordon de nodos; detección de nodo caído y reprogramación; roles de nodo (worker, edge).

Aceptación: apagar un nodo y ver las réplicas reprogramadas en < 60 s; las cargas de micro-VM no se asignan a nodos sin KVM.

## Documentación recomendada a dominar:

Algoritmos de Scheduling: Conceptos del Programador (Scheduler) en Español (filtrado, puntuación, afinidad y anti-afinidad).

Estrategias de Reprogramación y Drain: Safely Drain a Node Guide y patrones de bin-packing vs spread.

Plan diario de trabajo (S7)

D1: Modelo de datos del cluster de nodos y roles (worker, edge) en el control plane. D2: Algoritmo de filtrado de nodos basado en recursos disponibles y capacidades (KVM). D3: Algoritmos de puntuación de nodos: estrategias de bin packing y spread. D4: Soporte para afinidad y anti-afinidad de workloads por etiquetas y zonas de fallo. D5: Gestión de múltiples réplicas de un workload distribuidas entre diferentes nodos. D6: Implementación de comandos de cordon y drain ordenado de nodos para mantenimiento. D7: Detector de caída de nodos por timeout de latidos en el control plane. D8: Reprogramación automática de réplicas huérfanas en nodos sanos en menos de 60 segundos. D9: Pruebas de scheduling forzando cargas micro-VM en clusters mixtos (con y sin KVM). D10: Simulación de fallo abrupto de un worker y validación de convergencia de réplicas.

## S8 · Red entre nodos

Entregables: malla WireGuard automática entre nodos; DNS interno y descubrimiento de servicios; aislamiento por proyecto con nftables; política de egress por workload (permitir, denegar, allowlist); edge multinodo con certificados compartidos y DNS-01 para wildcards.

Aceptación: servicios de dos nodos se resuelven por nombre; un proyecto no puede alcanzar a otro; wildcard emitido correctamente.

## Retos de ingeniería (arquitectura §18):

Autoridad única de certificados: módulo certs en el control plane (CertMagic o lego) que emite, renueva con 30 días de antelación y guarda los certificados cifrados.

Distribución y edges sin estado: certificados y claves cifrados para cada nodo por el stream gRPC; Caddy con ACME desactivado.

Retos ACME: HTTP-01 reenviado al control plane por la malla WireGuard; DNS-01 para wildcards con credenciales cifradas como secretos; TLS-ALPN-01 desactivado.

Límites de Let's Encrypt: cola de emisión con límite de ritmo y backoff, un wildcard para los subdominios de la plataforma (previews incluidos) y CA alternativa como respaldo.

Criterio adicional: añadir un nodo edge no genera nuevas emisiones; con el control plane caído los edges siguen sirviendo el último certificado; prueba de caos con una renovación en curso.

## Documentación recomendada a dominar:

WireGuard: WireGuard Technical Whitepaper y WireGuard Quickstart Guide.

nftables en Linux: nftables Official Wiki & Quick Reference.

Lego / CertMagic (Go ACME): lego Documentation (Let's Encrypt Client) y CertMagic GitHub Documentation.

Plan diario de trabajo (S8)

D1: Automatización de interfaces WireGuard y gestión de claves públicas por nodo. D2: Topología de malla completa WireGuard entre nodos con MTU optimizada. D3: Servidor DNS interno integrado para resolución de nombres de servicio entre nodos. D4: Reglas de aislamiento de tráfico inter-proyecto mediante conjuntos y tablas de nftables. D5: Implementación de políticas de egress granular por workload (allowlist, deny, all). D6: Módulo certs en control plane con CertMagic/lego para emisión centralizada de certificados. D7: Validación DNS-01 para certificados wildcard de la plataforma y distribución por gRPC a edges. D8: Reenvío de retos HTTP-01 por el túnel WireGuard al control plane para dominios personalizados. D9: Pruebas de caos: caída temporal del control plane verificando que los edges no cortan TLS. D10: Validación de aislamiento de red entre proyectos distintos y emisión wildcard exitosa.

## S9 · Observabilidad y dashboard en vivo

Entregables: métricas de nodo y carga (/proc, cgroups) con gráficas; ingest de logs por lotes con compresión, almacenamiento por segmentos, búsqueda y retención; exportación Prometheus/OpenTelemetry; alertas básicas (nodo caído, disco, despliegue fallido) a email, Slack y webhooks; vista de eventos y auditoría.

Aceptación: 50 workloads emitiendo logs sin pérdida; consulta de logs < 1 s en la ventana reciente.

## Documentación recomendada a dominar:

Prometheus & OpenTelemetry: Prometheus Overview & Architecture y OpenTelemetry Documentation.

Linux cgroups v2 & procfs: Linux Kernel Control Group v2 Documentation.

Plan diario de trabajo (S9)

D1: Pipeline de recolección de métricas de CPU, memoria, disco y red desde /proc y cgroups v2. D2: Ingest de métricas en el control plane con almacenamiento en series temporales compactadas. D3: Exposición de métricas del sistema en formato Prometheus y OpenTelemetry. D4: Agente de recolección de logs por lotes con compresión zstd antes del envío. D5: Almacenamiento de logs segmentado por fecha y proyecto con índice invertido para búsquedas. D6: Motor de consultas de logs con filtrado en tiempo real en menos de 1 segundo. D7: Sistema de alertas (nodo caído, fallo de despliegue, saturación de disco) y despachador. D8: Canales de notificación: webhooks, mensajes de Slack y correo electrónico. D9: Gráficas interactivas de uso de CPU y memoria añadidas a las vistas del dashboard React. D10: Prueba de estrés con 50 workloads emitiendo logs simultáneos sin pérdida ni retraso.

## S10 · Bases de datos 1-click y backups — Beta privada v0.5

Entregables: catálogo Postgres, MySQL, Redis y MongoDB; volúmenes locales con afinidad de nodo; backups programados a S3 (cifrados, con retención); restauración con un clic y prueba de restauración automática periódica; actualizaciones de versión menores; credenciales como secretos.

Aceptación: crear DB, hacer backup, destruir y restaurar en otro nodo sin pérdida; invitar a 5-10 usuarios beta.

## Documentación recomendada a dominar:

PostgreSQL Backup & Recovery: PostgreSQL Backup and Restore Documentation y WAL archiving.

Amazon S3 API / Compatibilidad MinIO: Guía del usuario de Amazon S3 en Español y API de objetos.

Plan diario de trabajo (S10)

D1: Catálogo de plantillas de base de datos para PostgreSQL, MySQL, Redis y MongoDB. D2: Aprovisionamiento de volúmenes locales persistentes con fijación de afinidad al nodo. D3: Generación automática y almacenamiento seguro de credenciales de conexión en secretos. D4: Motor de backups programados ejecutando dumps consistentes en caliente. D5: Cifrado de backups con age y subida directa por streaming a almacenamiento compatible con S3. D6: Flujo de restauración de base de datos en un solo clic desde el dashboard. D7: Tarea periódica automatizada de prueba de restauración en entorno aislado para validar backups. D8: Procedimiento automatizado para actualizaciones de versión menor de bases de datos. D9: Prueba e2e: creación de base de datos, inserción de datos, backup, destrucción y restore. D10: Demostración de Beta privada v0.5 e incorporación de los primeros 10 usuarios de prueba.

5. Fase 3 — Micro-VMs y sandboxes (S11-S14) · Beta de sandboxes

## S11 · Runtime de micro-VMs y guest agent

Entregables: runtime-microvm con Cloud Hypervisor (o el hipervisor elegido en S0) tras el trait común; kernel guest y rootfs base reproducibles (images/); conversión imagen OCI → rootfs (ext4/virtiofs); interfaz TAP y red por VM; guest agent por vsock para exec, terminal, logs, métricas y archivos; límites de CPU/RAM/disco; selección de runtime en el Workload.

Aceptación: desplegar una app en micro-VM con terminal y logs; arranque en frío < 500 ms medido; sin SSH.

## Retos de ingeniería (arquitectura §18):

Política de aislamiento que falla cerrada: el Workload declara isolation: required o preferred; con required nunca baja a contenedor y, si ningún nodo tiene micro-VMs, queda Unschedulable con la razón visible en el dashboard; con preferred puede caer a contenedor con aviso visible.

Capacidades en la UI: insignias por nodo (contenedores, Wasm, micro-VM) con la causa cuando falta alguna.

KVM en caliente: revalidación periódica; un nodo que pierde KVM deja de recibir micro-VMs sin afectar a las existentes.

Desarrollo y CI: runtime fake para pruebas sin KVM y matriz de proveedores verificada en un CI semanal; gVisor como aislamiento intermedio opcional, documentado como no equivalente al de hardware.

Criterio adicional: una carga required sin nodo con KVM queda Unschedulable con su razón; ninguna carga se degrada en silencio.

## Documentación recomendada a dominar:

Cloud Hypervisor: Cloud Hypervisor REST API Reference y Virtio Devices Guide.

Linux vsock & virtio-vsock: vsock(7) Linux Programmer's Manual y sockets AF_VSOCK.

gVisor Sandbox Alternative: gVisor Architecture & Guide.

Plan diario de trabajo (S11)

D1: Trait común de runtime en el agente e integración de Cloud Hypervisor. D2: Pipeline de compilación de kernel guest mínimo y generación de rootfs ext4 reproducible. D3: Conversión de capas de imágenes OCI a sistemas de archivos rootfs utilizables por micro-VMs. D4: Configuración de red para micro-VMs mediante interfaces TAP, bridge y asignación IP. D5: Desarrollo del guest agent en Rust compilado estáticamente con musl para correr en el guest. D6: Protocolo de comunicación host-guest sobre sockets vsock para ejecución de comandos y métricas. D7: Streaming de terminal interactiva y logs del guest hacia el agente mediante vsock. D8: Implementación de política de aislamiento required vs preferred en el scheduler. D9: Medición de tiempos de arranque en frío de micro-VMs (< 500 ms) y verificación de seguridad sin SSH. D10: Despliegue de aplicación real en micro-VM con acceso web y terminal en vivo.

## S12 · Snapshots, pools y API de sandboxes

Entregables: snapshot y restore de memoria y disco; pool de micro-VMs precalentadas por plantilla; objeto Sandbox (TTL obligatorio, plantilla, recursos, egress); API para crear, ejecutar comandos, subir y descargar archivos, pausar, reanudar, clonar y destruir; destrucción automática por TTL e inactividad.

Aceptación: sandbox listo en < 50 ms desde pool/snapshot; 100 sandboxes concurrentes en un nodo de referencia sin fugas de recursos.

## Documentación recomendada a dominar:

Instant VM Boot & Memory Snapshots: Firecracker Snapshotting Documentation y gestión de diff-snapshots.

Linux userfaultfd & Copy-on-Write: userfaultfd(2) Linux Manual Page para carga perezosa de memoria.

Plan diario de trabajo (S12)

D1: Implementación de API en el hipervisor para volcado de memoria y estado a disco (snapshot). D2: Mecanismo de restauración instantánea de micro-VM desde snapshot de memoria y disco diff. D3: Optimización de carga perezosa de memoria con userfaultfd para acelerar el arranque. D4: Gestor de pools de micro-VMs precalentadas en reposo listas para asociar a workloads. D5: Definición del objeto y API Sandbox (plantilla base, límites de CPU/RAM, TTL, egress). D6: Endpoints de API para ejecutar comandos, pausar, reanudar y clonar sandboxes. D7: Operaciones de subida y descarga de archivos mediante vsock hacia el filesystem de la micro-VM. D8: Servicio de recolección de basura y destrucción forzada por expiración de TTL o inactividad. D9: Benchmark de arranque: validación de < 50 ms para sandboxes nacidos de pool/snapshot. D10: Prueba de concurrencia: 100 sandboxes activos simultáneamente en un nodo sin fugas.

## S13 · SDKs, CLI e intérprete de código para IA

Entregables: SDKs TypeScript, Python y Go; CLI (login, deploy, logs, sandbox run); plantilla code interpreter (Python y Node con bibliotecas comunes); política de egress por defecto denegada con allowlist; límites de tiempo, CPU y memoria; ejemplos de integración con agentes (llamada a herramienta que ejecuta código); documentación de la API.

Aceptación: un agente de ejemplo ejecuta código no confiable en un sandbox, sin acceso a red ni al host; pruebas de intento de escape documentadas.

## Documentación recomendada a dominar:

CLI Building con Cobra (Go): Cobra CLI Library Documentation.

Code Interpreter & Sandbox Security: E2B Sandboxing for AI Agents Docs y directrices de ejecución de código no confiable.

Plan diario de trabajo (S13)

D1: Generación y empaquetado del SDK oficial de TypeScript/JavaScript para la API de sandboxes. D2: Implementación y publicación de los SDKs para Python y Go. D3: Desarrollo de la CLI unificada en Go con Cobra (comandos login, deploy, logs, status). D4: Comando sandbox run interactivo en la CLI con streaming bidireccional de I/O. D5: Creación de la imagen base code-interpreter con entornos Python 3 y Node.js con librerías de IA. D6: Aplicación estricta de regla nftables de egress denegado por defecto con allowlist configurable. D7: Limitador rígido de tiempo de CPU y memoria para ejecución de código no confiable. D8: Ejemplos de integración de tool call con frameworks de agentes de IA (LangChain, LlamaIndex). D9: Pruebas de seguridad y suites de intentos de escape de sandbox documentadas. D10: Demo: agente de IA ejecutando código arbitrario en un sandbox sin acceso al host ni a la red.

## S14 · Previews por PR y builds aislados — Beta de sandboxes v0.8

Entregables: entorno efímero por Pull Request (URL propia, comentario automático en el PR, destrucción al hacer merge o cerrar); builds ejecutados dentro de micro-VMs sin acceso a secretos de producción; bases de datos de prueba desechables clonadas desde snapshot; cuotas de previews por proyecto.

Aceptación: abrir un PR crea preview en < 2 min y cerrarlo libera todos los recursos; un build malicioso de prueba no alcanza la red interna.

## Documentación recomendada a dominar:

GitHub Apps & Webhooks: Creación de aplicaciones de GitHub (Español) y API de Checks/Deployments.

Aislamiento de Builds CI/CD: SLSA Framework Specification para builds aislados y herméticos.

Plan diario de trabajo (S14)

D1: Controlador de eventos de Pull Request de GitHub para orquestación de entornos efímeros. D2: Generación de URLs de preview por PR con certificados wildcard automáticos. D3: Publicación y actualización automática de comentarios informativos en el PR de GitHub. D4: Reubicación de los procesos de build para ejecutarse dentro de micro-VMs totalmente aisladas. D5: Aislamiento estricto de credenciales: bloqueo total de secretos de producción en micro-VMs de build. D6: Clonado rápido de bases de datos de prueba desechables mediante snapshots CoW de volumen. D7: Destrucción automática de todos los recursos del preview al cerrarse o fusionarse el PR. D8: Sistema de cuotas máximas de previews concurrentes por proyecto para control de costes. D9: Prueba de build hostil: código que intenta escanear la red interna y verificar su contención. D10: Lanzamiento de Beta de sandboxes v0.8 con documentación de SDKs y CLI.

6. Fase 4 — Wasm, empresa y alta disponibilidad (S15-S18)

## S15 · WebAssembly de primera clase

Entregables: runtime-wasm y wasm-worker (proceso aislado con cgroups, seccomp y Landlock); componentes Wasm como artefactos OCI en el registry; enrutamiento HTTP hacia funciones (WASI HTTP); pooling allocator, límites de combustible/memoria/tiempo; soporte de Rust, Go (TinyGo), JS y Python hacia componentes; métricas y logs integrados.

Aceptación: arranque en frío de función < 5 ms; el worker no puede leer certificados ni sockets del agente.

## Documentación recomendada a dominar:

WASI & Wasmtime Embedding: WASI Official Site y Embedding Wasmtime Guide.

Landlock & Seccomp en Linux: Landlock LSM Documentation y seccomp(2) Linux Manual Page.

Plan diario de trabajo (S15)

D1: Crate runtime-wasm con motor Wasmtime embebido y soporte del estándar WASI. D2: Arquitectura de proceso separado wasm-worker para aislar la ejecución del agente principal. D3: Aislamiento a nivel de SO del worker usando cgroups v2, perfiles seccomp y Landlock LSM. D4: Soporte para empaquetar y almacenar componentes Wasm como artefactos OCI en Zot. D5: Implementación de WASI HTTP para enrutamiento directo de peticiones a funciones Wasm. D6: Configuración del pooling allocator de Wasmtime para reutilización ultra-rápida de instancias. D7: Mecanismo de limitación por consumo de combustible (fuel), cuota de memoria y timeout. D8: Verificación de compilación de componentes desde Rust, Go (TinyGo) y TypeScript. D9: Pruebas de seguridad: comprobar que el worker no puede acceder a sockets ni certificados. D10: Medición de arranque en frío < 5 ms y demostración de microservicios Wasm.

## S16 · Multi-equipo, cuotas y medición

Entregables: organizaciones y equipos con roles personalizables; SSO (OIDC y SAML opcional) y 2FA obligatorio opcional; cuotas por equipo (CPU, RAM, disco, sandboxes, previews); medición de uso con exportación y base para facturación; auditoría exportable; rate limiting por token.

Aceptación: un equipo que excede su cuota recibe un error claro y no afecta a otros equipos.

## Documentación recomendada a dominar:

OpenID Connect & SAML 2.0: OpenID Connect Core Specification y Qué es SAML (Guía Auth0 en Español).

Rate Limiting & Token Bucket: Estrategias y técnicas de limitación de frecuencia (Google Cloud Español).

Plan diario de trabajo (S16)

D1: Modelo jerárquico de organizaciones, equipos y membresías en la base de datos. D2: Sistema RBAC ampliado con permisos granulares y roles personalizados por organización. D3: Integración de inicio de sesión único (SSO) mediante proveedores OpenID Connect (OIDC). D4: Soporte preliminar para SAML 2.0 y política obligatoria de segundo factor (2FA). D5: Motor de cuotas por equipo (límite de vCPUs, RAM, almacenamiento, micro-VMs y previews). D6: Interceptores en el control plane para bloqueo estricto con error explicativo al superar cuotas. D7: Servicio de medición y contabilidad de recursos en el tiempo para facturación y reportes. D8: Exportación de eventos de auditoría y métricas de consumo en formato CSV y JSON. D9: Rate limiting token bucket por token de API y dirección IP para evitar abuso del control plane. D10: Pruebas de estrés de aislamiento multi-tenant y verificación de cuotas concurrentes.

## S17 · Alta disponibilidad del control plane

Entregables: múltiples réplicas del control plane detrás de un balanceador; PostgreSQL replicado (guía con Patroni o servicio gestionado) y PgBouncer; elección de líder para tareas únicas; agentes que se reconectan a cualquier réplica; backup y restauración de base de datos, CA y secretos; pruebas de failover; actualización continua del control plane.

Aceptación: matar una réplica o el primario de Postgres sin pérdida de datos y con recuperación < 60 s; las cargas en ejecución no se interrumpen.

## Documentación recomendada a dominar:

Patroni & HA en PostgreSQL: Patroni Documentation & Architecture y PgBouncer Connection Pooler Guide.

Algoritmos de Consenso & Leader Election: Raft Consensus Algorithm Guide.

Plan diario de trabajo (S17)

D1: Configuración del control plane en modo stateless detrás de balanceador de carga. D2: Integración con cluster de PostgreSQL de alta disponibilidad y pool de conexiones PgBouncer. D3: Implementación de mecanismo de elección de líder (vía Postgres leases/advisory locks). D4: Adaptación de tareas de fondo periódicas (cron, alertas, auditoría) para ejecutarse solo en el líder. D5: Manejo de reconexión transparente de streams gRPC de los agentes ante caída de una réplica. D6: Proceso redundante de CA y sincronización de certificados y secretos compartidos. D7: Simulación de caída del nodo primario de PostgreSQL y medición del tiempo de failover (< 60 s). D8: Procedimiento de actualización continua (rolling update) del control plane sin desconexión. D9: Validación de que las cargas de trabajo existentes en los workers continúan operando sin cortes. D10: Simulacro completo de disaster recovery con restauración de base de datos y CA.

## S18 · Escalado vertical y autoscaling

Entregables: redimensionado en caliente de CPU y memoria (hotplug en micro-VMs, límites cgroups en contenedores); autoscaling horizontal por CPU, memoria o RPS con cooldown; scheduler con bin packing mejorado, NUMA y hugepages; overcommit configurable con límites seguros; escalado a cero opcional para Wasm y previews.

Aceptación: una app escala de 1 a N réplicas bajo carga y vuelve a 1; una micro-VM duplica memoria sin reinicio.

## Documentación recomendada a dominar:

KVM/Virtio Memory Hotplug: Linux Memory Hotplug Documentation y virtio-balloon.

Autoscaling Horizontal & Cero: Knative Pod Autoscaler (KPA) & Scale to Zero Guide.

Plan diario de trabajo (S18)

D1: Redimensionamiento dinámico de cgroups v2 para contenedores en caliente sin parada. D2: Integración de virtio-balloon y memory hotplug en Cloud Hypervisor para micro-VMs. D3: CPU hotplug en micro-VMs para añadir vCPUs en tiempo real según demanda. D4: Algoritmo de autoscaling horizontal basado en métricas de CPU, memoria y requests por segundo. D5: Períodos de cooldown y estabilización en el autoscaler para mitigar el efecto flapping. D6: Scheduler optimizado con topología NUMA y afinidad a memoria HugePages para alto rendimiento. D7: Políticas de overcommit de memoria y CPU configurables con límites seguros de protección OOM. D8: Escalado a cero para funciones Wasm y entornos de preview inactivos. D9: Reactivación ultra-rápida desde cero al recibir tráfico HTTP en el edge. D10: Prueba de carga validando escalado de 1 a 20 réplicas y reducción suave a 1.

7. Fase 5 — Endurecimiento y lanzamiento (S19-S22)

## S19 · Seguridad y cadena de suministro

Entregables: revisión del modelo de amenazas; fuzzing de parsers (protocolo, vsock, rootfs); pruebas de escape de contenedor, micro-VM y Wasm; SBOM, firma de todos los binarios e imágenes, builds reproducibles; escaneo de dependencias (cargo deny, govulncheck); revocación de certificados verificada; endurecimiento de systemd y del instalador; pentest externo contratado.

Aceptación: sin hallazgos críticos ni altos abiertos; informe del pentest con plan de corrección.

## Documentación recomendada a dominar:

Fuzzing & Auditoría: cargo-fuzz Guide (Rust), Go Vulnerability Management (govulncheck) y cargo-deny Docs.

SBOM & Supply Chain: CycloneDX SBOM Standard y Guía de Seguridad en la Cadena de Suministro (INCIBE Español).

Plan diario de trabajo (S19)

D1: Revisión exhaustiva del modelo de amenazas y vector de ataque con todo el equipo. D2: Implementación de fuzzing continuo con cargo-fuzz en los parsers de vsock y protocolos binarios. D3: Fuzzing de parsers de configuración y deserializadores en Go y Rust. D4: Ejecución de suites de prueba de escape de contenedores, micro-VMs y entorno Wasm. D5: Generación automatizada de SBOM en formato CycloneDX en cada pipeline de build. D6: Firma criptográfica de todos los binarios, imágenes de contenedor y scripts con Cosign. D7: Bloqueo en CI de dependencias vulnerables mediante cargo-deny y govulncheck. D8: Endurecimiento de unidades systemd (protect system, private tmp, capabilities reducidas). D9: Inicio y coordinación de la auditoría de pentest externo sobre la plataforma completa. D10: Recepción del informe preliminar de seguridad y triaje de mitigaciones inmediatas.

## S20 · Rendimiento, carga y caos

Entregables: pruebas de carga con k6 (500 nodos simulados, miles de workloads, picos de logs); pruebas soak de 72 h; chaos testing (caída de nodos, particiones de red, disco lleno, reinicios del control plane); optimización de los cuellos de botella detectados (ingest de logs, gateway de agentes); definición y publicación de SLOs y alertas asociadas; runbooks de incidentes.

Aceptación: cumplir los presupuestos del documento de arquitectura bajo carga; runbooks ensayados en un simulacro.

## Documentación recomendada a dominar:

Pruebas de Carga con k6: Grafana k6 Documentation y metodologías de soak testing.

SRE & SLOs: Google Site Reliability Engineering (SRE Book) y The Site Reliability Workbook: Implementing SLOs.

Plan diario de trabajo (S20)

D1: Creación del banco de pruebas de carga con k6 simulando 500 agentes y tráfico masivo. D2: Inicio de la prueba de estabilidad soak de 72 horas continuas bajo carga constante. D3: Inyección de caos con caída programada de nodos y corte de enlaces de red WireGuard. D4: Pruebas de estrés en almacenamiento simulando disco lleno y cuellos de botella en I/O. D5: Optimización del ingest de logs por lotes y reducción del uso de CPU en el gateway de agentes. D6: Definición formal de los Service Level Objectives (SLOs) de latencia y disponibilidad. D7: Configuración de alertas de agotamiento de presupuesto de error (error budget burn rate). D8: Redacción de runbooks detallados de respuesta a incidentes críticos de infraestructura. D9: Cierre exitoso de la prueba soak de 72 h: verificación de ausencia de fugas de memoria o sockets. D10: Simulacro de incidente en equipo validando la efectividad práctica de los runbooks.

## S21 · Documentación, actualizaciones y beta pública — RC v0.95

Entregables: sitio de documentación (docs-site/), guías de inicio rápido, referencia de API y SDK, tutoriales; compatibilidad N-1 y actualizaciones por oleadas de agentes con rollback; migraciones de datos probadas entre versiones; telemetría opcional (opt-in); página de estado; programa de beta pública con canal de soporte.

Aceptación: un usuario nuevo instala y despliega su primera app solo con la documentación; actualización de la versión anterior sin caída.

## Documentación recomendada a dominar:

Sitios de Documentación y Diátaxis: Diátaxis Documentation Framework y Docusaurus Documentation.

Compatibilidad N-1 & Rolling Updates: Performing a Rolling Update Guide y versionado semántico de APIs.

Plan diario de trabajo (S21)

D1: Estructuración del sitio de documentación oficial docs-site/ bajo metodología Diátaxis. D2: Redacción de guías de inicio rápido paso a paso: desde cero a aplicación desplegada. D3: Documentación de referencia completa de la API ConnectRPC y especificaciones de SDKs. D4: Implementación de estrategia de compatibilidad hacia atrás N-1 en protocolos y APIs. D5: Sistema de actualizaciones escalonadas (canary/rolling) de agentes con rollback automático. D6: Verificación de migraciones de base de datos bidireccionales entre versiones consecutivas. D7: Implementación de telemetría opcional anónima (opt-in) para mejoras del producto. D8: Despliegue de la página pública de estado del servicio para transparencia operativa. D9: Apertura del canal de soporte comunitario y programa de beta pública para usuarios externos. D10: Tagging y publicación de Release Candidate v0.95 con validación de despliegue guiado.

## S22 · Candidato final y GA 1.0

Entregables: bug bash y cierre de errores bloqueantes; congelación de la API estable (buf breaking obligatorio desde aquí); notas de versión y guía de migración; licencias y términos; canal de soporte y proceso de divulgación de vulnerabilidades (SECURITY.md); anuncio, repositorio público y publicación de binarios firmados.

Aceptación: lista de verificación de producción (sección 8) completa; GA 1.0.

## Documentación recomendada a dominar:

Vulnerability Disclosure: OWASP Vulnerability Disclosure Cheat Sheet y estándar RFC 9116 (security.txt).

Checklist de Producción y Lanzamiento: Google SRE Launch Checklist y preparación para GA.

Plan diario de trabajo (S22)

D1: Jornada de bug bash intensivo con todo el equipo enfocado en casos límite y UX. D2: Triaje, priorización y resolución de errores bloqueantes detectados durante la beta. D3: Congelación oficial de la API v1.0 y activación estricta de comprobación buf breaking en CI. D4: Redacción de notas de la versión GA 1.0 y guías de migración desde las versiones beta. D5: Auditoría legal y de licencias de dependencias, definición de términos y condiciones de uso. D6: Publicación formal de SECURITY.md, security.txt (RFC 9116) y canal de reporte responsable. D7: Verificación minuciosa de cada uno de los puntos de la checklist de producción (Sección 8). D8: Publicación de binarios finales firmados, imágenes OCI inmutables e instaladores v1.0. D9: Apertura del repositorio público en GitHub, publicación del anuncio oficial y demo pública. D10: Celebración del lanzamiento GA 1.0, inicio de soporte formal y entrada en ciclo de guardia.

8. Lista de verificación para producción

Seguridad

mTLS en todos los canales, certificados de 24 h con rotación verificada.

Secretos cifrados en reposo; sin secretos en logs.

Binarios, imágenes y scripts firmados; SBOM publicado.

Pentest externo sin hallazgos críticos/altos; fuzzing continuo en CI.

Egress denegado por defecto en sandboxes; TTL obligatorio.

Fiabilidad

Control plane en HA probado con failover.

Backups de DB, CA y secretos con restauración probada en el último mes.

Reconciliación tras desconexión y reinicios verificada.

Actualizaciones graduales y rollback probados.

Rendimiento

Presupuestos cumplidos: agente < 50 MB, arranque de micro-VM < 500 ms (< 50 ms con snapshot), Wasm < 5 ms.

Pruebas de carga y soak de 72 h superadas.

Operación

SLOs, alertas, página de estado y runbooks.

Rotación de guardia definida y proceso de incidentes con postmortem sin culpa.

Política de versiones, soporte y fin de vida.

Producto

Documentación completa, SDKs publicados, instalación verificada en los proveedores soportados (lista oficial con y sin KVM).

9. Riesgos y mitigaciones por fase

Riesgo

Fase

Mitigación

KVM no disponible en VPS comunes

S0, S11

Detección de capacidades; lista de proveedores compatibles; plataforma útil sin micro-VMs

Dos toolchains (Go y Rust) frenan la velocidad

S1-S6

Contratos Protobuf, CI unificado, propiedad clara por componente

Complejidad de red (WireGuard, nftables, TAP)

S8, S11

Spike temprano en S0; pruebas e2e de red en CI

Ingest de logs como cuello de botella

S9, S20

Módulo aislado con backend intercambiable (ClickHouse/Loki)

Escapes de sandbox

S11-S15, S19

Micro-VM para código no confiable, fuzzing, pentest externo, revisión de modelo de amenazas

Alcance excesivo

Todas

Hitos con demo; recortar Wasm o SAML antes que la seguridad o la HA

Dependencia de un solo desarrollador por componente

Todas

Revisión cruzada obligatoria; documentación y ADRs

10. Después de GA (hoja de ruta posterior)

v1.1: replicación de volúmenes entre nodos y almacenamiento distribuido opcional.

v1.2: células multi-región con plano de gestión global.

v1.3: marketplace de plantillas y runtimes de sandbox; GPU en micro-VMs.

v2.0: opción de nube gestionada (SaaS) construida sobre la misma plataforma.

11. Estructura completa del repositorio

La siguiente estructura refleja el monorepo unificado del proyecto, diseñado para mantener una separación estricta de responsabilidades entre el plano de control (Go), el agente de nodo (Rust), las micro-VMs, los workers de WebAssembly, la interfaz web y las herramientas auxiliares:

.github/                    # Automatización de CI/CD, linters, escaneos de seguridad y release

├── workflows/            # Pipelines de GitHub Actions (ci.yml, release.yml, soak.yml)

└── CODEOWNERS            # Propietarios de código obligatorios (revisión estricta en proto/)

proto/                      # Especificaciones gRPC/Connect con Protocol Buffers y Buf

├── buf.yaml              # Configuración de linting, breaking change detection y plugins

├── buf.gen.yaml          # Configuración de generación de código (Go, Rust y TypeScript)

├── v1/                   # Esquemas versión 1

│   ├── agent.proto       # Comunicación de control plane a agente de nodo

│   ├── workload.proto    # Especificación declarativa de workloads y sandboxes

│   ├── auth.proto        # Autenticación, usuarios, tokens y sesiones

│   └── metrics.proto     # Telemetría, métricas y streaming de logs

control-plane/              # Plano de control principal (Go)

├── cmd/server/           # Punto de entrada principal del servidor ConnectRPC

├── internal/             # Lógica interna y servicios del negocio

│   ├── api/              # Handlers ConnectRPC generados y validados

│   ├── auth/             # Autenticación, RBAC, tokens API y OIDC/SAML

│   ├── certs/            # Autoridad certificadora ACME interna y wildcard (CertMagic/lego)

│   ├── db/               # Consultas generadas con sqlc y migraciones (Postgres / SQLite)

│   ├── scheduler/        # Programador multi-nodo con bin packing, spread y afinidad

│   └── billing/          # Medición de uso, cuotas por organización y rate limiting

└── go.mod                # Dependencias Go fijadas con directiva toolchain

agent/                      # Agente de ejecución del nodo (Rust, Tokio)

├── Cargo.toml            # Dependencias del workspace de Rust fijadas con rust-toolchain.toml

├── src/                  # Código fuente del daemon principal

│   ├── main.rs           # Inicialización, mTLS, latidos y conexión gRPC saliente

│   ├── reconciler/       # Máquina de estados declarativa para convergencia de workloads

│   ├── caddy/            # Adaptador y generador de configuración para API de Caddy

│   ├── network/          # Configuración de WireGuard, TAP, iptables/nftables y vsock

│   └── capabilities/     # Detección de KVM niveles 0 a 3, cgroups v2 y recursos

├── crates/               # Crates modulares independientes

│   ├── runtime-containerd/ # Integración con containerd vía Unix domain socket

│   ├── runtime-microvm/    # Conector Cloud Hypervisor / Firecracker sobre KVM

│   └── runtime-wasm/       # Runtime Wasmtime embebido con pooling allocator

guest-agent/                # Agente mínimo para el interior de las micro-VMs (Rust musl)

└── src/                  # Servidor vsock para exec, logs, métricas y transferencia

wasm-worker/                # Proceso aislado para ejecución Wasm (Rust Landlock/seccomp)

└── src/                  # Ejecutor de componentes WASI HTTP con límites de fuel

builder/                    # Servicio de construcción de imágenes OCI

├── buildkit/             # Integración con BuildKit para Dockerfiles con caché

└── buildpacks/           # Integración con Cloud Native Buildpacks para stacks detectados

images/                     # Recetas de compilación de sistemas base reproducibles

├── kernel/               # Configuración y compilación del kernel Linux guest minimalista

├── rootfs-base/          # Generación del rootfs ext4 base con guest-agent incluido

└── code-interpreter/     # Plantillas con entornos preinstalados para agentes de IA

web/                        # Dashboard SPA (React 19, TypeScript, Vite, Tailwind, shadcn/ui)

├── src/                  # Componentes de UI, enrutador TanStack y queries Connect-ES

└── package.json          # Dependencias web (embebido en el binario Go vía go:embed)

cli/                        # Herramienta de línea de comandos del desarrollador (Go, Cobra)

├── cmd/                  # Comandos: login, deploy, logs, status, sandbox run

└── main.go               # Punto de entrada del ejecutable cliente

sdk/                        # Librerías cliente para desarrolladores y agentes de IA

├── typescript/           # SDK oficial para Node.js y navegadores

├── python/               # SDK oficial para Python (LangChain, LlamaIndex)

└── go/                   # SDK oficial para aplicaciones en Go

deploy/                     # Despliegue, empaquetado y orquestación de producción

├── systemd/              # Archivos de servicio systemd endurecidos

├── caddy/                # Plantillas y esquemas base para Caddy

└── zot/                  # Configuración del registro OCI embebido

tests/                      # Suites de pruebas automáticas e integración

├── e2e/                  # Pruebas de extremo a extremo Go↔Rust contra contratos

├── fuzz/                 # Fuzzing de parsers vsock y protocolos (cargo-fuzz)

└── load/                 # Scripts de prueba de carga con k6 y escenarios de caos

docs/                       # Documentación y arquitectura Diátaxis (docs-site/)

├── adr/                  # Architecture Decision Records (ADR-001 a ADR-010+)

└── guides/               # Guías paso a paso de instalación, operación y seguridad

scripts/                    # Scripts de administración, instalación y verificación

├── install.sh            # Instalador en un comando del control plane (firmado)

└── install-agent.sh      # Instalador y enrolador del agente de nodo con token

Makefile                    # Automatización de tareas de compilación (make build, make proto)

SECURITY.md                 # Política de seguridad y proceso de divulgación de vulnerabilidades


