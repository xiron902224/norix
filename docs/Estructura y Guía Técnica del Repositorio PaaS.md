# Estructura y Guía Técnica del Repositorio PaaS

Documento convertido automáticamente desde DOCX.

<!-- Fuente: Estructura y Guía Técnica del Repositorio PaaS.docx -->

Estructura Completa, Explicación y Guía de Documentación del Repositorio PaaS

Este documento detalla de manera exhaustiva el árbol del repositorio, la descripción de cada archivo y componente, el propósito funcional de cada uno y los enlaces directos a la documentación oficial recomendada para completarlos e implementarlos con éxito.

Resumen Ejecutivo de Arquitectura

Dominio

Tecnología Base

Responsabilidad Primaria

Plano de Control

Go, ConnectRPC, SQLite/PostgreSQL

Orquestación global, reconciliación central, API de administración y emisión de certificados.

Agente de Nodo

Rust, Tokio, cgroups v2

Daemon de supervisión local, aprovisionamiento de interfaces de red y ejecución de contenedores.

Runtimes de Ejecución

containerd, Cloud Hypervisor, Wasmtime

Aislamiento multinivel: OCI estándar, micro-VMs con aislamiento por hardware y WebAssembly serverless.

Contratos e Interfaz

Protocol Buffers, Buf, React 19, Connect-ES

## Especificación de esquemas tipados de extremo a extremo, consola web y tooling CLI.

1. Directorio Raíz y Configuración Global

.github/

Propósito: Automatización de la integración continua (CI), despliegue (CD) y políticas de gobernanza del repositorio.

## Archivos clave:

.github/workflows/ci.yml: Pipeline principal de pruebas unitarias, linting y compilación cruzada en arquitecturas x86_64 y aarch64.

📚 Documentación recomendada: GitHub Actions Workflow Syntax.

.github/workflows/release.yml: Pipeline para empaquetado de binarios, firmas con Cosign e publicación de releases.

📚 Documentación recomendada: Sigstore Cosign Documentation.

.github/CODEOWNERS: Define los revisores obligatorios por rutas del monorepo (especialmente estricto en la carpeta proto/).

📚 Documentación recomendada: GitHub Codeowners Guide.

Makefile

Propósito: Automatizador central para las tareas de desarrollo, compilación, generación de stubs gRPC y validación de linting.

Contenido: Comandos estandarizados (make build, make proto, make test, make lint).

📚 Documentación recomendada: GNU Make Manual.

SECURITY.md

Propósito: Política de seguridad formal del proyecto, canales de reporte responsable de vulnerabilidades y estándar de divulgación.

Contenido: Instrucciones para reportar fallos de seguridad (compatible con RFC 9116 / security.txt).

📚 Documentación recomendada: OWASP Vulnerability Disclosure Cheat Sheet.

2. Definiciones de Contratos y API

proto/

Propósito: Contrato único e interoperable de la plataforma basado en Protocol Buffers, utilizado tanto por el plano de control en Go como por los agentes en Rust y el dashboard en TypeScript.

## Archivos clave:

proto/buf.yaml: Configuración de reglas de linter estricto y control de compatibilidad (breaking changes).

📚 Documentación recomendada: Buf CLI Configuration.

proto/buf.gen.yaml: Configuración de plugins de generación de código multiprogramación (Go, Rust, Connect-ES).

📚 Documentación recomendada: Buf Code Generation Overview.

proto/v1/agent.proto: Definición del stream gRPC bidireccional de control, latidos y reporte de estado observado del nodo.

📚 Documentación recomendada: Protocol Buffers Language Guide.

proto/v1/workload.proto: Especificación declarativa de workloads, contenedores, micro-VMs, funciones Wasm, despliegues multi-contenedor vía Docker Compose y enrutamiento.

📚 Documentación recomendada: ConnectRPC Protocol Specification y Docker Compose Specification.

proto/v1/auth.proto: Contratos de autenticación, sesiones, tokens de API y RBAC.

proto/v1/metrics.proto: Esquemas para telemetría, ingesta de métricas de rendimiento y streaming de logs.

3. Plano de Control (Go)

control-plane/

Propósito: Monolito modular en Go responsable de la orquestación global, recepción de APIs, gestión de estado deseado y autorizaciones.

## Archivos y subdirectorios clave:

control-plane/go.mod: Definición de dependencias y directiva de versión de Go.

control-plane/cmd/server/main.go: Punto de entrada del binario del control plane, inicialización de servidores ConnectRPC y embebido del dashboard con go:embed.

📚 Documentación recomendada: Go embed Package y ConnectRPC Go Getting Started.

control-plane/internal/api/: Handlers ConnectRPC para dashboard, CLI y SDK; autenticación y OIDC.

control-plane/internal/auth/: Lógica de RBAC, cifrado de claves con Argon2id, gestión de tokens API con SHA-256 y aislamiento granular de variables de entorno (evitando compartir secretos masivamente entre servicios de un Compose).

control-plane/internal/certs/: Módulo de CA interna X.509, emisión y renovación automática de certificados mTLS y wildcard (CertMagic/lego).

📚 Documentación recomendada: CertMagic Go Documentation.

control-plane/internal/db/: Esquemas SQL, migraciones y código fuente generado mediante sqlc para SQLite y PostgreSQL.

📚 Documentación recomendada: sqlc Documentation.

control-plane/internal/scheduler/: Algoritmo de colocación de cargas con bin-packing, spread, afinidad y verificación de capacidades de nodo (KVM).

📚 Documentación recomendada: Kubernetes Scheduler Concepts (Referencia algorítmica).

control-plane/internal/billing/: Medición de uso de recursos, límites por equipos y limitación de frecuencia (rate limiting).

4. Agente de Nodo y Runtimes (Rust)

agent/

Propósito: Daemon ligero ejecutado como servicio systemd en cada nodo VPS, escrito en Rust sin GC (basado en Tokio) para máxima eficiencia.

## Archivos y subdirectorios clave:

agent/Cargo.toml y agent/rust-toolchain.toml: Fijación de versiones de compilación y dependencias del workspace.

agent/src/main.rs: Inicialización de Tokio, enrolamiento inicial del nodo mediante token de un solo uso y apertura del stream gRPC saliente con mTLS.

📚 Documentación recomendada: Tokio Tutorial & Guide.

agent/src/reconciler/: Máquina de estados local que asegura la convergencia continua entre el estado deseado y el observado.

agent/src/caddy/: Adaptador de control de la API REST de Caddy para enrutamiento dinámico sin cortes, soporte de dominios comodín (wildcards) y gestión de Edge.

📚 Documentación recomendada: Caddy Admin API Guide.

agent/src/network/: Automatización de interfaces WireGuard, túneles TAP, configuración de nftables y descubrimiento DNS interno.

📚 Documentación recomendada: WireGuard Whitepaper y nftables Wiki.

agent/src/capabilities/: Detección precisa de características de hardware del host: KVM (niveles 0 a 3), cgroups v2, CPU, RAM y disco.

agent/crates/runtime-containerd/: Integración nativa con containerd a través de sockets de dominio UNIX para contenedores OCI individuales y pilas multi-contenedor de Docker Compose.

📚 Documentación recomendada: containerd Go/Rust API Docs.

agent/crates/runtime-microvm/: Orquestador de micro-VMs de aislamiento de hardware mediante Cloud Hypervisor (o Firecracker).

📚 Documentación recomendada: Cloud Hypervisor REST API Reference.

agent/crates/runtime-wasm/: Runtime embebido basado en Wasmtime con pooling allocator para ejecución de componentes Wasm en microsegundos.

📚 Documentación recomendada: Wasmtime Guide & Docs.

guest-agent/

Propósito: Agente mínimo compilado estáticamente con musl para ejecutarse dentro del sistema de archivos invitado (guest) de las micro-VMs.

Contenido: Servidor ligero de comunicación por sockets vsock para comandos de ejecución remota (exec), terminal, transmisión de logs y métricas sin necesidad de SSH.

📚 Documentación recomendada: Linux vsock(7) Manual Page.

wasm-worker/

Propósito: Proceso aislado ejecutor de funciones WebAssembly segregado del agente principal.

Contenido: Worker dotado de restricciones estrictas de seguridad a nivel de sistema operativo mediante cgroups v2, perfiles seccomp y Landlock LSM.

📚 Documentación recomendada: Landlock LSM Documentation.

5. Builder e Imágenes Base

builder/

Propósito: Servicio de construcción de imágenes OCI y procesamiento de fuentes a partir de repositorios Git de los usuarios.

## Componentes:

builder/buildkit/: Integración con BuildKit para compilación eficiente de Dockerfiles con caché de capas.

📚 Documentación recomendada: BuildKit Architecture & Guide.

builder/buildpacks/: Integración con Cloud Native Buildpacks para detección automática de stacks en repositorios sin Dockerfile.

📚 Documentación recomendada: Buildpacks Official Documentation.

builder/static/: Módulo de procesamiento rápido de carpetas y archivos HTML/CSS/JS estáticos puros sin requerir pasos de compilación (issue de HTML estático).

images/

Propósito: Recetas, scripts y definiciones para la construcción de sistemas base reproducibles de la plataforma.

## Subdirectorios:

images/kernel/: Configuración y compilación del kernel Linux guest minimalista optimizado para Cloud Hypervisor.

images/rootfs-base/: Generación del sistema de archivos raíz (rootfs ext4) con el guest-agent integrado.

images/code-interpreter/: Plantillas con entornos preinstalados para ejecución segura de sandboxes de IA.

6. Frontend y Herramientas de Cliente

web/

Propósito: Panel de control web (SPA) para la gestión visual de proyectos, despliegues, bases de datos y terminales interactivas.

Stack: React 19, TypeScript, Vite, TanStack Router, TanStack Query, Tailwind CSS y componentes shadcn/ui (embebido en el binario Go mediante go:embed).

## Subdirectorios:

web/src/: Componentes de UI, estados locales con Zustand y cliente Connect-ES generado desde los contratos .proto.

web/package.json: Definición de dependencias frontend y scripts de compilación estática.

📚 Documentación recomendada: Vite Guide y TanStack Router Docs.

cli/

Propósito: Herramienta de línea de comandos oficial (paas) para desarrolladores.

## Subdirectorios:

cli/cmd/: Implementación de comandos con Cobra (login, deploy, logs, status, sandbox run, subida de carpetas estáticas).

cli/main.go: Punto de entrada del binario ejecutable de la CLI.

📚 Documentación recomendada: Cobra CLI Library Documentation.

sdk/

Propósito: Librerías oficiales cliente para desarrolladores y orquestación externa mediante agentes de IA.

## Subdirectorios:

sdk/typescript/: SDK oficial para Node.js y navegadores.

sdk/python/: SDK oficial para Python (ideal para integraciones con LangChain y LlamaIndex).

sdk/go/: SDK oficial para aplicaciones escritas en Go.

📚 Documentación recomendada: ConnectRPC Web/TS Guide.

7. Despliegue, Pruebas y Documentación

deploy/

Propósito: Artefactos de despliegue, empaquetado y orquestación para entornos de producción.

## Subdirectorios:

deploy/systemd/: Archivos de unidad systemd altamente endurecidos para servicios del control plane y el agente.

deploy/caddy/: Plantillas de configuración base para el proxy inverso Caddy con soporte de comodines y variables de servidor.

deploy/zot/: Ficheros de configuración para el registro OCI interno embebido Zot.

📚 Documentación recomendada: Systemd Service Unit Options.

tests/

Propósito: Batería integral de pruebas automatizadas, conformidad y caos de la plataforma.

## Subdirectorios:

tests/e2e/: Pruebas de extremo a extremo que validan la interoperabilidad entre componentes Go y Rust frente a los stubs generados.

tests/fuzz/: Pruebas de fuzzing continuo en los deserializadores de protocolos y sockets vsock (cargo-fuzz).

📚 Documentación recomendada: cargo-fuzz Guide.

tests/load/: Scripts de prueba de carga con k6 y escenarios de inyección de caos.

📚 Documentación recomendada: Grafana k6 Documentation.

docs/

Propósito: Sitio de documentación oficial de la arquitectura y guías operativas bajo metodología Diátaxis.

## Subdirectorios:

docs/adr/: Architecture Decision Records (ADR-001 a ADR-010+), documentando formalmente cada decisión de diseño técnico.

docs/guides/: Guías paso a paso de instalación, operación, seguridad y resolución de problemas.

📚 Documentación recomendada: Diátaxis Documentation Framework.

scripts/

Propósito: Scripts auxiliares de aprovisionamiento, instalación automatizada y diagnóstico.

## Archivos clave:

scripts/install.sh: Script en una línea para instalación limpia del plano de control en un VPS, firmado criptográficamente.

scripts/install-agent.sh: Script para despliegue y enrolamiento automático del agente de nodo mediante token de un solo uso.

📚 Documentación recomendada: Bash Best Practices (Google Shell Style Guide).
