# Threat model (S19) — STRIDE corto

| Amenaza | Superficie | Mitigación v1 |
|---|---|---|
| Spoofing | Join tokens, sesiones | Tokens de un uso; sesión opaca hasheada; rotar join tokens |
| Tampering | Desired state / edge config | Solo CP escribe; Caddy load + health rollback |
| Repudiation | Mutaciones | audit_events con actor |
| Information disclosure | Logs/API errors | No filtrar SQL; secretos AES; RLS |
| Denial of service | Login/bootstrap | Rate limit; lockout path |
| Elevation of privilege | RBAC | requireMutator; roles owner/admin/member |

Pentest checklist: RLS cross-tenant, path traversal en logs, SSRF en webhooks, privilege escalation en team invites.

## Docker socket (`/var/run/docker.sock`) — #10

**Amenaza:** montar el socket de Docker en un contenedor otorga el poder del daemon (en instalación rootful ≈ root del host): crear contenedores privilegiados, montar `/`, etc.

**Trade-off Norix:**

| Diseño | Blast radius | UX one-box |
|---|---|---|
| Socket en **control-plane** (legacy embedded runtime) | Compromiso del panel/API → host | Simple, peor seguridad |
| Socket solo en **agent** (default Compose) | Compromiso del agent → host; CP sin socket | Un servicio más + join token |
| Socket proxy / rootless | Reduce o acota impacto; no elimina necesidad de API amplia para builds | Ops extra |

**Mitigación v1:** Compose de producción **no** monta el socket en el control-plane; el servicio `agent` sí. `NORIX_EMBEDDED_RUNTIME=1` + override `docker-compose.embedded-runtime.yml` es opt-in documentado. Installer advierte y enlaza [`docs/user/docker-socket.md`](./user/docker-socket.md).

**Fuera de alcance v1:** exigir proxy en el path default (el agent necesita create/build/start). Rootless y proxy quedan documentados como hardening opcional.
