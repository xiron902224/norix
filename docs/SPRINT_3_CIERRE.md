# Sprint 3 — Agente Rust, enrolamiento y heartbeat

## Objetivo

Un nodo aparece vivo en el control plane: join token → enroll → heartbeat con reconexión.

## Estado

**Implementado el 2026-11-10.** Siguiente: S4 (reconciliación + runtime contenedores).

## Entregables

- Proto `AgentService`: CreateJoinToken, EnrollNode, Heartbeat, ListNodes, ConnectStream.
- Migración `000004`: tablas `join_tokens` y `nodes` con RLS.
- Control plane: auth de sesión (UI) + credencial de nodo (agent RPCs); stale nodes → offline (~45s).
- Agente Rust (`agent/`): detección de capacidades, enroll, heartbeat + backoff, estado local.
- UI Nodes: crear join token, copiar, listar nodos + `last_seen`.
- `scripts/install-agent.sh` (build from source / release bin).
- CI: job Rust (`fmt`, `clippy`, `build --release`) + verificación de tablas `nodes`/`join_tokens`.

## DoD

| Criterio | Estado |
|---|---|
| Nodo visible en dashboard tras enroll | Cumple |
| Heartbeat actualiza `last_seen` / status online | Cumple |
| Fallo de red → backoff y reintento | Cumple (agente) |
| Agente release build | Cumple (`cargo build --release`) |
| Deploy sin contenedores aún | Esperado: FailedPrecondition hasta S4 si hay nodo |

## Notas

- MVP de identidad: token de nodo + TLS del transport (HTTP local); mTLS/PKI completo puede endurecerse en S8.
- `ConnectStream` está implementado en el control plane; el agente S3 usa Heartbeat unary (suficiente para el DoD de presencia).
- Deploy sigue bloqueado hasta reconciler de contenedores (S4).
