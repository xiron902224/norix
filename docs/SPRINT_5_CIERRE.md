# Sprint 5 — Git, build y registry

## Objetivo

git URL (o push webhook) → clone → Dockerfile build → imagen → deploy.

## Estado

**Implementado el 2026-11-10.** Siguiente: S6 (edge/SSL / Alpha 0.1).

## Entregables

- Proto: `DeployFromGit`, `ListBuilds`, `GetBuildLogs`; agent `DesiredBuild` / `ReportBuild`.
- Migración `000007`: `builds`, `build_logs`, `build_secrets`, `webhook_tokens` + columnas git en workloads.
- Control plane: crea builds, heartbeat entrega desired builds, al éxito actualiza imagen y hace Deploy.
- Local runtime + agente Rust: `git clone`, `docker/nerdctl build`, push opcional, logs.
- Registry local **zot** en `deploy/norix` (`127.0.0.1:5000`).
- Webhook GitHub: `POST /hooks/github/{token}` + mint `POST /api/webhooks/workloads/{id}`.
- UI: formulario Build & deploy from git, Rebuild, build logs drawer.
- Ejemplo: `examples/hello-go`.
- ADR-003: secretos de build AES-GCM (`NORIX_BUILD_SECRETS_KEY`).

## DoD

| Criterio | Estado |
|---|---|
| Repo con Dockerfile → imagen → running | Cumple (localruntime o agent + docker) |
| Logs de build en UI | Cumple |
| Registry zot en Compose | Cumple (push best-effort si el daemon acepta insecure registry) |
| Webhook GitHub | Cumple (token por workload) |
| Secretos cifrados en reposo | Cumple (opcionales; requieren key) |

## Notas

- Si el push a zot falla (registry insecure no configurado), se usa el tag local del mismo daemon Docker.
- Buildpacks quedan fuera de S5 (solo Dockerfile).
- Para demos locales: `docker compose -f deploy/norix/docker-compose.yml up -d postgres registry`.
