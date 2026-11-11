# Arquitectura final de la plataforma PaaS

Documento convertido automáticamente desde DOCX.

<!-- Fuente: Arquitectura final de la plataforma PaaS.docx -->

## Arquitectura final de la plataforma PaaS

PaaS self-hosted con contenedores, micro-VMs y WebAssembly. Documento de arquitectura v1.0.

1. Visión y objetivos

Construir una plataforma de despliegue self-hosted, instalable en una línea, que ofrezca la experiencia de Coolify (git push, SSL automático, bases de datos 1-click) y añada tres capacidades diferenciadoras:

Aislamiento por hardware con micro-VMs (KVM) para cargas no confiables.

WebAssembly de primera clase para funciones y handlers de arranque ultrarrápido.

Sandboxes efímeros programables (API y SDK) para agentes de IA, previews por PR y bases de datos de prueba.

Objetivos medibles (presupuestos)

Métrica

Objetivo

RAM del agente en reposo

< 50 MB

RAM del control plane (1 nodo, SQLite)

< 150 MB

Dashboard en producción

0 procesos Node; archivos estáticos servidos por el binario de Go

Arranque en frío de micro-VM

< 500 ms (< 50 ms con snapshot y pool precalentado)

Arranque en frío de función Wasm

< 5 ms

Nodos por control plane

500+ con un stream gRPC por agente

Instalación del control plane

< 3 minutos en un VPS limpio

No objetivos de la v1

Almacenamiento distribuido de bloques (los volúmenes son locales al nodo, con backup a S3).

Orquestación compatible con Kubernetes.

Multi-región activa-activa del control plane.

2. Principios de diseño

Estado deseado declarativo. El control plane publica el estado deseado versionado; el agente reconcilia y reporta el estado observado. Nada de comandos imperativos frágiles.

El agente siempre inicia la conexión (stream saliente). Funciona detrás de NAT y no exige abrir puertos en los nodos.

Un artefacto, tres runtimes. Todo se construye como artefacto OCI y se materializa según el motor elegido.

Aislamiento explícito. Cada carga declara su nivel de aislamiento; la UI y la API lo muestran.

Monolito modular primero. Se divide en servicios solo cuando una métrica lo justifique.

Capacidades detectadas, no supuestas. El agente reporta qué puede hacer (docker, wasm, microvm) y el scheduler decide.

Seguridad por defecto: mTLS, egress denegado en sandboxes, secretos cifrados, binarios firmados.

3. Stack tecnológico final

Capa

Tecnología

Motivo

Control plane

Go

Concurrencia, binario estático, ecosistema de gRPC y Docker/containerd

Agente de nodo

Rust (Tokio)

Sin GC, huella mínima, integración natural con Wasmtime y Cloud Hypervisor

Micro-VMs

Cloud Hypervisor (detrás de una interfaz común; Firecracker como alternativa evaluable)

Hotplug, virtio-fs, escrito en Rust

Wasm

Wasmtime, en procesos worker aislados

El pooling allocator da arranques de microsegundos

Contenedores

containerd directo (sin dockerd)

Menor consumo y menos superficie

Build

BuildKit + Cloud Native Buildpacks

Compatible con Dockerfile y detección automática

Registry

zot (OCI nativo, ligero)

Un binario, soporta artefactos Wasm

Edge / proxy

Caddy (controlado por API)

ACME integrado; se elige uno solo, no Traefik

Base de datos

SQLite (un nodo) o PostgreSQL (HA) con sqlc

Misma capa de acceso para ambos

API

Protobuf + ConnectRPC (gRPC, gRPC-Web y JSON desde un solo contrato)

Tipos únicos para Go y TypeScript

Frontend

Vite + React 19 + TypeScript (SPA estática)

Ver sección 4

Red entre nodos

WireGuard + nftables

Malla cifrada y políticas de egress

Observabilidad

OpenTelemetry, Prometheus, almacenamiento de logs propio

Estándares abiertos

4. Frontend: React sin SSR

Decisión: el dashboard es una SPA de React construida con Vite, exportada como archivos estáticos y embebida en el binario del control plane con go:embed.

¿Por qué no Next.js con SSR? Un servidor Node en producción consume cientos de MB de RAM, añade un proceso más que instalar y mantener, y no aporta nada a un panel autenticado que no necesita SEO. Con la SPA estática el dashboard cuesta 0 MB de RAM adicional y se actualiza junto con el binario.

## Stack del dashboard:

Vite + React 19 + TypeScript (build rápido, salida estática).

TanStack Router (rutas con tipos seguros) y TanStack Query (caché de datos y reintentos).

Cliente ConnectRPC generado con buf desde los mismos .proto que usa Go: los tipos nunca se desincronizan.

Tailwind CSS + shadcn/ui (componentes accesibles y sin runtime pesado).

xterm.js para la terminal web y WebSocket/streaming para logs y métricas en vivo.

Zustand para estado de UI local; Vitest + Playwright para pruebas.

Alternativas válidas si en el futuro hace falta SSR: TanStack Start o React Router v7 (modo framework). Next.js queda reservado para el sitio público de marketing y documentación (docs-site/), desplegable como estático, fuera del binario.

5. Arquitectura general

Usuario / Git push / SDK / CLI              │              ▼ ┌─────────────────────────────────────────────────────────────┐ │ CONTROL PLANE (Go, monolito modular, N réplicas en HA)      │ │  ├─ API Connect/gRPC + dashboard estático embebido          │ │  ├─ Auth, RBAC, equipos, auditoría, cuotas                  │ │  ├─ Scheduler + reconciliador (estado deseado versionado)   │ │  ├─ CA interna (PKI), secretos cifrados                     │ │  ├─ Cola de builds + webhooks Git                           │ │  └─ Gateway de streams de agentes                           │ └───────┬─────────────────────────────────────┬───────────────┘         │ PostgreSQL / SQLite                  │ Registry OCI (zot)         ▼                                      ▼   Estado + eventos                  Imágenes y componentes Wasm   ▲  stream gRPC saliente, mTLS (iniciado por cada agente)   │ ┌─┴───────────────────────────────────────────────────────────┐ │ NODO (Agente Rust)                                          │ │  ├─ Reconciliador local + estado observado                  │ │  ├─ Runtimes (trait Runtime):                               │ │  │    containerd │ Cloud Hypervisor/Firecracker │ Wasm workers│ │  ├─ Red: WireGuard, TAP, nftables, DNS interno              │ │  ├─ Edge opcional: Caddy                                    │ │  ├─ Almacenamiento: volúmenes, rootfs, snapshots            │ │  └─ Telemetría: métricas, logs → OpenTelemetry              │ └─────────────────────────────────────────────────────────────┘

6. Control plane

Monolito modular con módulos internos de fronteras claras (cada uno podría extraerse a un servicio si se justifica):

api: handlers ConnectRPC para dashboard, CLI y SDK; autenticación por sesión, token de API y OIDC.

agentgw: gateway que mantiene un stream gRPC bidireccional por agente, entrega el estado deseado y recibe estado observado, eventos y métricas.

scheduler: coloca cargas según capacidades del nodo, recursos libres, afinidad, nivel de aislamiento y zonas. Estrategia inicial: bin packing con puntuación; luego spread configurable.

reconciler (lado servidor): compara estado deseado y observado, genera planes y reintenta con backoff.

builds: cola de trabajos, asignación a builders, caché de capas, publicación en el registry.

pki: CA interna, emisión de certificados de corta vida (24 h) por CSR, rotación y revocación.

secrets: cifrado con age (o KMS externo), versionado y entrega al agente cifrada para el nodo destino.

git: GitHub App, GitLab y Bitbucket; webhooks, clonado y detección de stack.

billing/quotas: medición de uso y límites por equipo.

audit: registro inmutable de acciones.

Persistencia: capa común con sqlc. SQLite en modo WAL para instalaciones de un nodo; PostgreSQL para HA. Migraciones con goose o atlas. Eventos importantes en una tabla outbox para entrega fiable.

Compatibilidad: el control plane soporta agentes de la versión N y N-1 para actualizaciones graduales. Las versiones del protocolo se negocian al conectar.

7. Agente de nodo (Rust)

## Proceso único, ligero, ejecutado como servicio systemd. Responsabilidades:

Enrolamiento: token de un solo uso con expiración → CSR → certificado mTLS de vida corta, con rotación automática.

Stream saliente gRPC hacia el control plane con reconexión y backoff exponencial.

Reconciliación local: si el nodo pierde conexión, sigue operando con el último estado deseado conocido y reconcilia al volver.

Detección de capacidades: /dev/kvm, virtualización anidada, versión de kernel, cgroups v2, CPU, RAM, disco; las reporta como capabilities.

Runtimes detrás de un trait común: create, start, stop, exec, logs, stats, snapshot, restore.

Red, almacenamiento y telemetría (secciones 9 y 11).

Runtimes

Runtime

Aislamiento

Uso recomendado

containerd (runc)

Namespaces + cgroups + seccomp

Apps del propio equipo, cargas de confianza

containerd + gVisor/Kata (opcional)

Kernel de usuario / VM ligera

Confianza media

Cloud Hypervisor / Firecracker

Hardware (KVM)

Código no confiable, multitenant, sandboxes de IA, builds

Wasmtime (worker)

Sandbox de Wasm + proceso aislado

Funciones, handlers HTTP, plugins

Wasm en workers separados: el agente lanza cada worker como un proceso sin privilegios, con cgroups, seccomp y Landlock, de modo que un escape de Wasmtime no comprometa el proceso que guarda las claves mTLS. Wasm se posiciona para funciones y handlers con WASI acotado, no para aplicaciones arbitrarias.

Micro-VMs: requieren /dev/kvm. En VPS sin virtualización anidada el nodo funciona solo con contenedores y Wasm; el scheduler no colocará micro-VMs allí. Las micro-VMs usan un kernel y rootfs base mantenidos por el proyecto, y un guest agent en Rust (estático, musl) que se comunica por vsock para exec, terminal, logs, métricas y transferencia de archivos, sin SSH.

Arranque rápido: snapshot/restore de memoria y un pool de micro-VMs precalentadas por plantilla.

8. Modelo declarativo y flujo de despliegue

## El contrato (en proto/) define objetos de estado deseado versionados:

Workload (imagen o componente, runtime, recursos, puertos, volúmenes, env, secretos, nivel de aislamiento, política de red).

Volume, Route (dominios y TLS), Sandbox (TTL, plantilla, egress) y NodeConfig.

## Flujo Git → producción:

Webhook o push llega al control plane.

Se encola un build; un builder aislado en micro-VM ejecuta BuildKit o Buildpacks.

La imagen OCI se publica en el registry.

Se crea una nueva revisión del Workload (estado deseado v+1).

El scheduler elige nodo; el agente descarga el artefacto y lo materializa según el runtime: contenedor, rootfs ext4/virtiofs para micro-VM, o componente Wasm.

Se actualiza la ruta del edge; el tráfico cambia con health checks (despliegue gradual o azul/verde) y rollback automático si falla.

El agente reporta el estado observado; el dashboard lo muestra en vivo.

9. Red

Malla WireGuard entre nodos con direccionamiento interno y descubrimiento de servicios por DNS interno.

Aislamiento por proyecto: cada proyecto en su propia red lógica; reglas nftables (no iptables).

Micro-VMs: interfaz TAP por VM, conectada a un bridge del nodo con reglas por VM.

Egress: denegado por defecto en sandboxes; allowlist por dominio o CIDR en la política del Sandbox.

Edge: cualquier nodo puede tener rol edge (Caddy). Los certificados se almacenan de forma compartida (en el control plane o almacén S3) para evitar conflictos de ACME; DNS-01 para wildcards; Caddy se configura por API desde el agente.

Balanceo: el edge distribuye hacia réplicas en distintos nodos por la malla interna.

10. Almacenamiento y datos

Volúmenes locales al nodo (ext4/XFS o ZFS/btrfs opcional para snapshots rápidos).

Bases de datos 1-click (Postgres, MySQL, Redis, MongoDB) como workloads con volumen; el scheduler las fija a su nodo.

Backups automáticos a S3 (cualquier endpoint compatible), con retención, cifrado y restauración probada periódicamente.

Snapshots de sandboxes y volúmenes para clonar entornos (previews y bases de prueba).

Futuro: replicación de volúmenes entre nodos y almacenamiento distribuido (Ceph/Longhorn) como complemento, sin cambiar el modelo Volume.

11. Observabilidad

Métricas: el agente expone y envía métricas de nodo (/proc, cgroups) y de cada carga; formato Prometheus/OpenTelemetry.

Logs: el agente captura stdout/stderr y los envía por lotes con compresión al ingest del control plane, con almacenamiento por segmentos y retención configurable (ClickHouse o Loki como backend opcional a gran escala). El ingest de logs es el primer cuello de botella previsto, por lo que se aísla como módulo extraíble.

Trazas: OpenTelemetry opcional.

Alertas: reglas básicas integradas (nodo caído, disco lleno, despliegue fallido) con notificación por email, Slack y webhooks.

Auditoría: quién hizo qué y cuándo.

12. Seguridad y modelo de amenazas

Amenaza

Mitigación

Código de un tenant escapa a otro tenant

Micro-VM por tenant no confiable; contenedores solo para cargas de confianza

Escape de Wasm hacia el agente

Workers separados, sin privilegios, con seccomp y Landlock

Nodo comprometido

Certificados de corta vida, revocación, secretos entregados solo al nodo que los necesita

Token de enrolamiento filtrado

Un solo uso, expiración corta, vinculación a IP opcional

Cadena de suministro (binarios, imágenes)

Firma con cosign/minisign, checksums, SBOM, builds reproducibles

Build malicioso

Builds dentro de micro-VMs sin acceso a secretos de producción

Exfiltración desde sandboxes de IA

Egress denegado por defecto, cuotas de CPU/RAM/tiempo, TTL obligatorio

Abuso de recursos

Cuotas por equipo, límites cgroups, rate limiting

Además: RBAC con equipos y roles, 2FA, tokens de API con alcance, cifrado de secretos en reposo, registro de auditoría y revisión periódica del modelo de amenazas.

13. Escalabilidad

Horizontal

Más nodos: cada agente es independiente y mantiene un solo stream; el gateway del control plane escala con réplicas detrás de un balanceador (el agente se reconecta a cualquiera).

Control plane sin estado de sesión: el estado vive en PostgreSQL; el líder para tareas únicas (reconciliación global, cron) se elige con advisory locks o lease.

Particionado futuro: cuando un solo control plane no baste, se particiona por región/clúster (una célula por región, con un plano de gestión global ligero).

Extracción de servicios: builds, ingest de logs y gateway de agentes pueden separarse en binarios independientes con el mismo contrato Protobuf, sin reescribir.

Edge escalable: más nodos con rol edge, DNS con múltiples registros o balanceador anycast.

Builders: pool horizontal de builders efímeros en micro-VMs, escalado según la cola.

Vertical

Aprovechar un solo nodo grande: Tokio multihilo en el agente, cgroups v2 con asignación de CPU/NUMA, hugepages para micro-VMs.

Redimensionado en caliente: hotplug de vCPU y memoria con Cloud Hypervisor y resize de límites cgroups sin reinicio, cuando el runtime lo permita.

Densidad: memory ballooning y overcommit configurable con límites seguros.

Base de datos: PostgreSQL con réplicas de lectura y connection pooling (PgBouncer) antes de cualquier fragmentación.

Almacenamiento: NVMe local y colas de E/S separadas por carga.

Evolución prevista

Un nodo, SQLite (instalación mínima).

Multi-nodo, un control plane.

Control plane en HA con Postgres replicado.

Logs y builds como servicios separados.

Células por región.

14. Instalación, actualización y operación

Control plane: curl -fsSL https://get.tu-plataforma.dev | sh instala el binario estático firmado y un servicio systemd. Soporta SQLite por defecto o Postgres externo.

Nodo agente: curl -fsSL https://get.tu-plataforma.dev/agent | sh -s -- --token <TOKEN> (nota: sh -s -- es necesario para pasar argumentos al script). Verifica firma y checksum, comprueba /dev/kvm, instala containerd y cloud-hypervisor si procede, y completa el enrolamiento.

Idempotencia: los scripts pueden ejecutarse varias veces; incluyen uninstall y upgrade.

Actualizaciones: del control plane con rollback automático; de agentes por oleadas (canary → resto), soportando N-1.

Backups del propio control plane: base de datos, CA y secretos, con procedimiento de recuperación documentado.

Modos de despliegue: un binario con systemd (recomendado), Docker Compose (desarrollo) y Helm opcional para ejecutar el control plane en Kubernetes.

15. Estructura completa del repositorio (monorepo)

/├── .github/│   ├── workflows/               # CI: lint, test, build multi-arch, release firmado, e2e con KVM│   ├── ISSUE_TEMPLATE/│   └── CODEOWNERS│├── proto/                       # Fuente única de verdad de contratos│   ├── buf.yaml                 # Lint y detección de cambios incompatibles│   ├── buf.gen.yaml             # Generación para Go, Rust y TypeScript│   ├── platform/v1/│   │   ├── api.proto            # API pública (dashboard, CLI, SDK)│   │   ├── workload.proto       # Workload, Volume, Route, Sandbox│   │   ├── agent.proto          # Stream agente ↔ control plane (estado deseado/observado)│   │   ├── metrics.proto        # Métricas y salud│   │   └── guest.proto          # Control plane/agente ↔ guest agent (vsock)│├── control-plane/               # Backend central en Go│   ├── cmd/│   │   ├── server/              # main.go del control plane│   │   └── ctl/                 # Utilidades administrativas (migrar, rotar CA, backup)│   ├── internal/│   │   ├── api/                 # Handlers ConnectRPC y middleware (auth, rate limit)│   │   ├── agentgw/             # Gateway de streams de agentes│   │   ├── scheduler/           # Colocación y puntuación de nodos│   │   ├── reconciler/          # Estado deseado vs observado│   │   ├── builds/              # Cola de builds, integración con builders│   │   ├── git/                 # Webhooks y proveedores (GitHub, GitLab, Bitbucket)│   │   ├── pki/                 # CA interna, emisión y revocación de certificados│   │   ├── secrets/             # Cifrado y entrega de secretos│   │   ├── auth/                # Sesiones, tokens, OIDC, 2FA, RBAC│   │   ├── billing/             # Medición de uso y cuotas│   │   ├── audit/               # Registro de auditoría│   │   ├── logs/                # Ingest y consulta de logs (extraíble)│   │   ├── db/│   │   │   ├── migrations/      # SQL versionado (SQLite y Postgres)│   │   │   ├── queries/         # Consultas para sqlc│   │   │   └── gen/             # Código generado por sqlc│   │   ├── gen/                 # Código generado por buf (Go)│   │   └── webui/               # Embebe web/dist con go:embed│   ├── go.mod│   └── go.sum│├── agent/                       # Demonio de nodo en Rust (workspace Cargo)│   ├── Cargo.toml│   └── crates/│       ├── agent/               # Binario principal (main.rs, runtime de Tokio)│       ├── core/                # Tipos, reconciliador local, estado│       ├── control-client/      # Cliente gRPC, enrolamiento, mTLS│       ├── runtime-api/         # Trait Runtime común│       ├── runtime-containerd/  # Contenedores vía containerd│       ├── runtime-microvm/     # Cloud Hypervisor y Firecracker (snapshots, pools)│       ├── runtime-wasm/        # Supervisor de workers Wasmtime│       ├── wasm-worker/         # Binario worker aislado (seccomp, Landlock)│       ├── net/                 # WireGuard, TAP, nftables, DNS interno│       ├── edge/                # Control de Caddy por API│       ├── storage/             # Volúmenes, rootfs, snapshots│       ├── telemetry/           # Métricas (/proc, cgroups), logs, OTel│       └── capabilities/        # Detección de KVM, cgroups, hardware│├── guest-agent/                 # Agente dentro de las micro-VMs (Rust, musl, vsock)│   ├── Cargo.toml│   └── src/│├── builder/                     # Servicio de build aislado│   ├── buildkit/                # Configuración de BuildKit│   ├── buildpacks/              # Builders y detección de stack│   └── vm-image/                # Imagen de micro-VM del builder│├── images/                      # Imágenes base para micro-VMs│   ├── kernel/                  # Configuración y build del kernel guest│   ├── rootfs/                  # Rootfs mínimos (alpine, debian-slim, python, node)│   └── templates/               # Plantillas de sandbox (code-interpreter, etc.)│├── web/                         # Dashboard: Vite + React + TypeScript (SPA estática)│   ├── index.html│   ├── vite.config.ts│   ├── tsconfig.json│   ├── package.json│   ├── public/│   └── src/│       ├── main.tsx│       ├── routes/              # TanStack Router (proyectos, nodos, sandboxes, builds, ajustes)│       ├── features/            # Módulos por dominio (deployments, databases, sandboxes...)│       ├── components/          # UI kit, terminal (xterm.js), gráficas, logs en vivo│       ├── api/                 # Cliente Connect generado + hooks de TanStack Query│       ├── stores/              # Zustand│       ├── lib/                 # Utilidades│       └── styles/│├── cli/                         # CLI del usuario (Go): login, deploy, logs, sandbox│├── sdk/                         # SDKs generados + helpers│   ├── typescript/│   ├── python/│   └── go/│├── deploy/│   ├── install.sh               # Instalador del control plane (firmado)│   ├── install-agent.sh         # Instalador del agente│   ├── systemd/                 # Units de servicio│   ├── docker-compose.yml       # Entorno de desarrollo local│   ├── helm/                    # Chart opcional│   └── terraform/               # Ejemplos para aprovisionar nodos│├── tests/│   ├── e2e/                     # Extremo a extremo (usa KVM en runners)│   ├── load/                    # Carga y soak (k6)│   ├── chaos/                   # Fallos de red, nodos y disco│   └── security/                # Fuzzing y pruebas de escape de sandbox│├── docs/                        # Documentación técnica (ADRs, runbooks, threat model)│   ├── adr/│   ├── runbooks/│   └── threat-model.md├── docs-site/                   # Sitio público (Next.js estático u otro SSG)│├── scripts/                     # Utilidades de desarrollo y release├── Makefile                     # make proto | build | test | dev | release├── Cross.toml / .golangci.yml / rustfmt.toml / deny.toml├── CONTRIBUTING.md  SECURITY.md  LICENSE  README.md└── CHANGELOG.md

Flujo de generación de código: make proto ejecuta buf generate y produce los stubs en control-plane/internal/gen, el crate del agente y web/src/api. make build compila el dashboard (web/dist), lo embebe en el binario de Go y compila el agente y el guest agent para x86_64 y aarch64.

16. Decisiones de arquitectura (resumen de ADRs)

#

Decisión

Alternativa descartada

Razón

1

SPA Vite + React estática embebida

Next.js con SSR

Sin proceso Node, RAM mínima, instalación más simple

2

Monolito modular en Go

Microservicios desde el inicio

Menos complejidad operativa; extracción posterior posible

3

Estado deseado declarativo

Comandos imperativos

Tolera desconexiones; idempotente

4

Agente inicia la conexión

Control plane conecta a nodos

Funciona tras NAT, sin puertos abiertos

5

containerd directo

dockerd

Menor consumo y superficie de ataque

6

Wasm en workers aislados

Wasmtime dentro del agente

Un escape no compromete claves ni al agente

7

Una sola tecnología de edge (Caddy)

Caddy y Traefik a la vez

Evita estado y ACME duplicados

8

SQLite o Postgres con sqlc

Solo Postgres

Instalación mínima y camino a HA

9

Contratos Protobuf + ConnectRPC

REST escrito a mano

Tipos únicos Go/Rust/TS

10

Interfaz común Cloud Hypervisor/Firecracker

Casarse con un hipervisor

Permite medir y cambiar con bajo coste

17. Riesgos principales

Virtualización anidada no disponible en muchos VPS: mitigación con detección de capacidades y documentación de proveedores compatibles.

Mantener dos toolchains (Go y Rust): mitigación con contratos Protobuf estrictos, CI unificado y propiedad clara por componente.

Alcance muy amplio: mitigación con fases (ver documento de sprints) y la API de sandboxes como cuña de mercado.

Seguridad de sandboxes: revisión externa y fuzzing antes de ofrecer multitenant.

Ingest de logs bajo carga: módulo aislado con backend intercambiable.

18. Retos de ingeniería a tener en cuenta durante el desarrollo

Tres retos pueden frenar el proyecto si se dejan para tarde: el toolchain dual Go + Rust, la falta de /dev/kvm en muchos VPS y la coordinación de certificados en los nodos edge con Caddy. Cada uno tiene aquí su análisis, la resolución decidida y el sprint donde se valida.

Reto

Qué falla si se ignora

Resolución

Se valida en

Toolchain dual (Go + Rust)

Contratos desincronizados, CI lento, builds que solo funcionan en una máquina

Contrato único con buf, frontera estrecha por Protobuf, CI por componente y convenciones estrictas de Tokio

S0, S3, S4

Virtualización anidada (/dev/kvm)

Micro-VMs que fallan en nodos sin KVM, o se degradan en silencio a contenedores

Detección por niveles, scheduler basado en capacidades y política de aislamiento que falla cerrada

S0, S3, S11

Ingress y Caddy multinodo

Límites de Let's Encrypt, certificados duplicados, configuración pisada entre escritores

Una sola autoridad de certificados (control plane), edges sin ACME y un único escritor de configuración por nodo

S6, S8

18.1 Complejidad del toolchain dual (Go + Rust)

Análisis. El coste no está en los lenguajes, sino en las costuras: tres lenguajes generados desde un mismo contrato (Go, Rust, TypeScript), compilación cruzada del agente (musl, x86_64 y aarch64), un CI que crece con cada componente y código asíncrono en Tokio que toca KVM, sockets Unix y la API de Cloud Hypervisor. Los fallos típicos son cambios de contrato que rompen al otro lado, tareas que bloquean el runtime y apagados que dejan procesos huérfanos.

Resolución.

Contrato único y verificado. buf lint y buf breaking contra main en cada PR; los stubs generados nunca se editan y el CI falla si make proto deja diferencias (git diff --exit-code). Los paquetes llevan versión (platform/v1), los campos eliminados se marcan reserved y los cambios de proto/ exigen dos revisores y un ADR.

Pruebas de conformidad entre lenguajes. Una suite en tests/e2e/ con un agente simulado en Go y un control plane simulado en Rust ejecuta los mismos casos contra ambos lados, de modo que el contrato se prueba aunque un componente aún no exista.

Frontera estrecha. Go y Rust solo se comunican por Protobuf sobre gRPC o vsock: sin FFI ni código compartido. El control plane decide y el agente ejecuta; el agente se mantiene con poca lógica de negocio.

CI por componente. Matriz con filtros por ruta (un cambio en web/ no compila Rust), cachés (sccache, cargo-chef, caché de Go y de buf), compilación cruzada con cargo-zigbuild o cross, versiones fijadas (rust-toolchain.toml, directiva toolchain de Go) y actualizaciones con Renovate. Objetivo: PR en menos de 10 minutos; las pruebas e2e completas y con KVM corren por la noche.

Calidad automatizada. clippy -D warnings, cargo deny, golangci-lint y govulncheck bloquean la fusión.

Reglas de Tokio para el agente.

Todo await externo lleva timeout y cancelación.

Canales acotados para tener backpressure; nunca colas sin límite.

Llamadas bloqueantes (ioctl de KVM, E/S de disco) en spawn_blocking, nunca en el runtime.

Apagado ordenado con CancellationToken y JoinSet.

Los sockets Unix (por ejemplo la API de Cloud Hypervisor) se encapsulan en un cliente tipado con reintentos y se prueban contra un servidor falso.

Todo unsafe vive en crates pequeños y con revisión obligatoria; tracing en todos los límites y tokio-console en desarrollo.

Personas. CODEOWNERS por componente y al menos dos personas que conozcan cada lado. Se reserva presupuesto de aprendizaje en S0-S3.

Plan de salida. La interfaz Runtime y el contrato Protobuf permiten sustituir piezas sin reescribir el sistema. Si tras S4 la fricción del toolchain consume una parte desproporcionada del tiempo del equipo, se reevalúa la opción de un agente en Go con Rust solo para wasm-worker y guest-agent. La decisión se documenta como ADR.

18.2 Soporte de virtualización anidada (/dev/kvm)

Análisis. Que exista /dev/kvm no garantiza que las micro-VMs funcionen: puede faltar el permiso, el proveedor puede exponer KVM con un rendimiento degradado o sin algunas funciones (snapshots, vsock), y el soporte puede desaparecer tras una migración del proveedor. El riesgo mayor no es que falle, sino que la plataforma se degrade sin avisar y entregue menos aislamiento del prometido.

Resolución.

## Detección por niveles en el crate capabilities del agente:

Nivel 0: sin /dev/kvm.

Nivel 1: /dev/kvm presente y accesible, con las banderas de CPU necesarias (vmx o svm).

Nivel 2: prueba real, que arranca una micro-VM mínima y verifica vsock, red y snapshot.

Nivel 3: medición corta de arranque y de E/S. El resultado se reporta como microvm: {available, reason, nested, boot_ms} y se revalida periódicamente.

El instalador informa, no falla. install-agent.sh muestra el resultado de la prueba con una explicación clara y enrola el nodo igualmente, con las capacidades que tenga (contenedores y Wasm).

Scheduler por capacidades. Una carga con isolation: microvm solo se coloca en nodos con micro-VM disponible. Si no hay ninguno, queda en estado Unschedulable con la razón visible en el dashboard.

Política que falla cerrada. El Workload declara isolation: required o preferred. Para código no confiable (sandboxes de IA, builds de terceros) es siempre required y nunca baja a contenedor. Con preferred (por ejemplo previews) puede usar contenedor, con aviso visible en la UI.

Topología mixta recomendada. Control plane y nodos de contenedores o Wasm en VPS económicos; nodos de micro-VM en bare-metal o en instancias con virtualización anidada verificada. El modelo multi-nodo ya lo permite.

Aislamiento intermedio opcional. gVisor como runtime adicional para nodos sin KVM; se documenta que no equivale al aislamiento por hardware.

Matriz de proveedores verificada. Una lista de proveedores y planes probados con la prueba de nivel 2 en un CI semanal; no se declara compatible nada que no haya pasado la prueba. Se arranca en S0 con 2-3 proveedores.

Desarrollo y CI. Runner con KVM para e2e y un runtime fake para pruebas unitarias y desarrollo en máquinas sin KVM.

18.3 Ingress y gestión de Caddy a escala

Análisis. Si cada nodo edge pide sus certificados por su cuenta aparecen cuatro problemas: solicitudes duplicadas que agotan los límites de Let's Encrypt, retos ACME que llegan a un nodo distinto del que los pidió, renovaciones que compiten entre sí y estado de certificados repetido en cada nodo. A eso se suma que la API de administración de Caddy aplica configuraciones completas, así que dos escritores concurrentes se pisan.

Resolución. Separar la emisión de certificados del servicio de TLS.

El control plane es la única autoridad de certificados (módulo certs, con CertMagic o lego como biblioteca). Emite, renueva y guarda cada certificado cifrado en la base de datos.

Distribución a los edges por el stream gRPC existente: certificado y clave cifrados para el nodo destino. Los Caddy corren con certificados cargados y ACME desactivado, por lo que los edges son prácticamente sin estado y se pueden añadir, drenar o reemplazar sin reemitir nada.

Retos ACME centralizados. HTTP-01: todos los edges reenvían /.well-known/acme-challenge/ al control plane por la malla WireGuard. DNS-01 para wildcards, con credenciales DNS cifradas como secretos. TLS-ALPN-01 desactivado.

Un solo escritor de configuración por nodo. Solo el agente toca la API de Caddy. Genera la configuración completa desde las Route deseadas (idempotente), la valida antes de aplicarla, la aplica sin interrupción y conserva la última válida para revertir. Si la versión de Caddy lo permite, usa control optimista de concurrencia con ETag; si no, serializa los cambios.

Protección de los límites de Let's Encrypt.

Cola de emisión con límite de ritmo y backoff.

Comprobar que el DNS del dominio apunta a un edge antes de pedir el certificado.

Un certificado wildcard para los subdominios generados por la plataforma (previews incluidos).

Servidor de staging en desarrollo y CI.

CA alternativa (por ejemplo ZeroSSL) como respaldo.

Resiliencia. Renovación con 30 días de antelación y reintentos; si el control plane cae, los edges siguen sirviendo el último certificado. Alertas a 14 días del vencimiento y métricas cert_expiry_seconds y acme_failures_total.

Fases. En S6 (un nodo) Caddy usa ACME nativo detrás de una interfaz CertProvider; en S8 se cambia a la emisión centralizada sin tocar el resto del código.

Pruebas. Pebble (servidor ACME de pruebas) en e2e para emisión, renovación y fallos; pruebas de caos con el control plane caído durante una renovación; verificación de que añadir un nodo edge no genera nuevas emisiones.

Compromiso aceptado. El control plane custodia claves privadas de TLS. Se mitiga con cifrado en reposo (age o KMS), entrega solo a los edges que las necesitan, rotación y registro de auditoría. La alternativa de Caddy con almacenamiento compartido y bloqueos distribuidos se descarta para producción porque acopla los edges al backend de almacenamiento y complica la alta disponibilidad.

18.4 Señales de alerta temprana

El PR medio supera los 15 minutos de CI o hay más de un contrato roto por mes: revisar la estructura del monorepo y las pruebas de conformidad.

Más del 30 % de los nodos candidatos no pasan la prueba de nivel 2 de KVM: reforzar la oferta de gVisor y la topología mixta antes de publicar sandboxes.

Más de una emisión por dominio y mes, o cualquier error de límite de Let's Encrypt en producción: auditar la cola de emisión del control plane.
