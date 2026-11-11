# Plan de Sprints Semanales - PaaS Next-Gen (Go + Rust + Next.js)

Documento convertido automáticamente desde DOCX.

<!-- Fuente: Plan de Sprints Semanales - PaaS Next-Gen (Go + Rust + Next.js).docx -->

Plan de Sprints Semanales - PaaS Next-Gen (Go + Rust + Next.js)

Resumen del Proyecto y Objetivos de Producción

Planes de desarrollo para llevar a producción una PaaS self-hosted ultra eficiente (<50MB RAM por nodo en reposo) con soporte para:

Micro-VMs mediante Cloud Hypervisor (KVM)

WebAssembly nativo mediante Wasmtime

Contenedores Docker/OCI tradicionales

Agentes edge distribuidos de latencia ultra baja vía gRPC sobre mTLS

Dashboard en Next.js (App Router con Server Components / SSR optimizado)

Componente

Stack Tecnológico

Rol en el Sistema

Control Plane

Go

Orquestación global, APIs, base de datos y webhooks

Edge Agent

Rust

Daemon ligero de nodo, métricas y gestión de runtimes

Micro-VM Engine

Cloud Hypervisor

Aislamiento a nivel de hardware por KVM

Wasm Engine

Wasmtime

Ejecución ultrarrápida de funciones de baja latencia

Dashboard UI

Next.js (App Router con SSR/SSG y RSC)

Panel de control centralizado y terminal interactivo

## Fase 1: Arquitectura Base y Comunicación (Sprints 1 - 3)

Sprint 1: Monorepo, Contratos gRPC e Instalador Base

Configuración del monorepo (control-plane/, agent/, web/, proto/).

Definición de contratos protobuf en proto/agent.proto y proto/metrics.proto (mensajes DeployRequest, StreamLogsRequest, NodeStatus).

Generación de código gRPC en Go y Rust.

Desarrollo del script inicial de instalación install.sh de 1 línea (detección de KVM, descarga de binarios).

Sprint 2: Control Plane Inicial (Go) & API Base

Implementación del servidor gRPC en Go para recibir conexiones de agentes.

Setup de base de datos PostgreSQL/SQLite y ORM para usuarios, proyectos y nodos.

Autenticación mTLS entre el Control Plane y los Agentes.

API REST/GraphQL inicial en Go para el dashboard en Next.js.

Sprint 3: Agente Base (Rust) & Conexión al Host

Implementación de la estructura del daemon en Rust usando el runtime tokio.

Conexión gRPC bidireccional estable del Agente hacia el Control Plane con reconexión automática.

Módulo de recolección de métricas del host (CPU, RAM, disco) leyendo /proc y cgroups.

Transmisión en tiempo real de la salud del nodo hacia Go.

## Fase 2: Motores de Ejecución y Aislamiento (Sprints 4 - 6)

Sprint 4: Motor Docker/OCI en Rust

Integración de la crate bollard en el agente para comunicarse con el socket de Docker/Podman.

Implementación de operaciones pull, build, run, stop y logs para contenedores estándar.

Rutas dinámicas para exponer contenedores a la red local.

Sprint 5: Motor Micro-VMs con Cloud Hypervisor (Rust)

Integración de la API HTTP de Cloud Hypervisor sobre socket UNIX (/run/cloud-hypervisor.sock).

Gestión de rootfs ext4 mínimos y asignación de CPU/RAM por Micro-VM.

Configuración de interfaces de red TAP y reglas de natting/iptables para las VMs.

Prueba de concepto de arranque de una Micro-VM en milisegundos a través de un comando gRPC.

Sprint 6: Motor WebAssembly con Wasmtime (Rust)

Integración de la crate wasmtime directamente dentro del binario del Agente.

Carga y aislamiento en memoria de binarios .wasm.

Implementación de servidor HTTP ligero interno en Rust para enrutar tráfico hacia funciones Wasm.

Medición de rendimiento y consumo de memoria (<10MB RAM).

## Fase 3: Networking, Proxies y Sandboxes Avanzados (Sprints 7 - 9)

Sprint 7: Ingress Proxy & Certificados SSL Automáticos

Integración del proxy inverso (Caddy/Traefik) dinámico controlado vía API desde el Agente.

Enrutamiento automático de dominios y subdominios hacia Contenedores, Micro-VMs y Wasm.

Automatización de certificados SSL/TLS con Let's Encrypt / ACME.

Sprint 8: Sandboxes Efímeros (AI Interpreters & PR Previews)

Implementación de lógica de limpieza y destrucción automática de VMs/Contenedores efímeros.

Creación de entornos aisados para ejecución de código IA (AI Code Interpreters).

Flujo de PR Preview Environments: despliegue automático por Pull Request y destrucción al cerrar el PR.

Sprint 9: Pipelines de Build y Webhooks de Git

Handler en el Control Plane (Go) para recibir webhooks de GitHub, GitLab y Bitbucket.

Motor de build distribuido: clonación del repo, detección de stack (Buildpacks/Dockerfile) y envío del artefacto al nodo de destino.

Gestión de variables de entorno cifradas en base de datos.

## Fase 4: Frontend Dashboard, UX & Polish para Producción (Sprints 10 - 12)

Sprint 10: Dashboard Principal (Next.js con SSR/RSC)

Desarrollo de la interfaz gráfica en Next.js (App Router, Tailwind CSS, TypeScript, React Server Components para reducir el bundle JS).

Vistas de gestión de proyectos, nodos conectados, bases de datos y sandboxes.

Autenticación de usuarios y roles de equipo.

Sprint 11: Real-time Terminal & Logs Viewer

Integración de consola terminal interactiva (xterm.js) en el frontend Next.js vía WebSockets/gRPC streaming.

Visualización de logs unificados en tiempo real para aplicaciones, VMs y funciones Wasm.

Gráficas de rendimiento (CPU, RAM, Red) en vivo por nodo y por aplicación.

Sprint 12: 1-Click Apps, Testing E2E & Hardening de Producción

Catálogo de bases de datos y plantillas 1-Click (Postgres, Redis, MySQL).

Pruebas E2E de despliegue completo en VPS limpios (Hetzner, DigitalOcean).

Auditoría de seguridad (cgroup limits, mTLS validation, KVM isolation check).

Publicación de la versión v1.0.0 y documentación oficial de producción.
