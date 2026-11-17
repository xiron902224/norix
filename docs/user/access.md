# Access & teams

**Access / Settings** manages who can use the organization:

- team members and roles (owner / admin / member as exposed in UI)
- SSO / OIDC providers when configured
- TOTP for accounts when enabled
- **Personal access tokens (PAT)** (#22) for CLI/API

## Personal access tokens

1. Open **Teams / Access** → **Personal access tokens**.
2. **Create token** (name e.g. `cli`) → copy the `norix_pat_…` value once.
3. Call the API with `Authorization: Bearer <token>` (same as a session cookie bearer).
4. **Revoke** when done; hashes are stored, plaintext never again.

Tokens inherit your user role. Optional expiry via API `expires_at` (RFC3339). Audit: `user.pat_created` / `user.pat_revoked`.

Invite collaborators at the **GitHub repo** level separately if they develop Norix itself; platform users only need a Norix login.

See [`AUTH_OIDC_TOTP.md`](../AUTH_OIDC_TOTP.md) for operator SSO setup.
