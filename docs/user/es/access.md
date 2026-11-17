# Acceso y equipos

**Access / Settings**: miembros, roles, SSO/OIDC y TOTP si está habilitado. Detalle operador: [`AUTH_OIDC_TOTP.md`](../../AUTH_OIDC_TOTP.md).

## Tokens de acceso personal (PAT)

1. **Teams / Access** → **Personal access tokens**.
2. **Create token** (nombre p.ej. `cli`) → copia el valor `norix_pat_…` (solo una vez).
3. Llama a la API con `Authorization: Bearer <token>`.
4. **Revoke** al terminar; en DB solo hay hash SHA-256.

Heredan el rol del usuario. Expiración opcional vía API (`expires_at`). Auditoría: `user.pat_created` / `user.pat_revoked`. Issue #22.
