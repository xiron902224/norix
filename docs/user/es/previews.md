# Entornos preview

Cada pull request puede desplegarse en una URL única (#21).

## Cómo funciona

1. Configura el control plane con org/proyecto y secretos de GitHub.
2. Añade un webhook `pull_request` a `POST /api/webhooks/github`.
3. Al abrir/sincronizar/reabrir, Norix crea o actualiza el workload `pr-<número>`, encola un build del SHA del PR, asigna el dominio `pr-<n>.<repo>.preview.localhost` y comenta la URL si hay `NORIX_GITHUB_TOKEN`.
4. Al cerrar (o por TTL) se marca destroyed y se borra el workload.

Variables: ver la [guía EN](../previews.md). DNS wildcard y Caddy hacen falta para HTTPS público.
