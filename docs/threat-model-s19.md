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
