# Auth: TOTP and OIDC

## TOTP (authenticator 2FA)

1. Sign in, open **Teams → SSO and 2FA → Enable TOTP**.
2. Scan or copy the secret into an authenticator app.
3. **Verify** with a current 6-digit code (marks TOTP enabled).
4. On the next login, after email/password the UI asks for the authenticator code.

Enforcement:

- User has TOTP enabled → `Login` requires `totp_code`.
- Organization `mandatory_2fa = true` → same, and the user must already have verified TOTP.

Secrets are AES-GCM encrypted with `NORIX_BUILD_SECRETS_KEY` (64 hex chars). The control plane **fails closed** at startup if the key is missing. Local/dev plaintext (`dev-nonce`) is only allowed when `NORIX_ALLOW_INSECURE_DEV_SECRETS=1` is set explicitly.

## OIDC SSO

HTTP endpoints on the control plane:

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/auth/oidc/providers` | Public list of enabled OIDC providers `{id,name}` |
| GET | `/auth/oidc/start?provider_id=` | Redirect to IdP |
| GET | `/auth/oidc/callback` | Code exchange → session → redirect to web |

### Environment

```bash
# Control plane public URL (used as OIDC redirect base)
NORIX_PUBLIC_URL=http://127.0.0.1:8080

# Where the browser lands after SSO (Vite or embedded UI)
NORIX_WEB_ORIGIN=http://127.0.0.1:5173

# Optional: encrypt TOTP secrets (recommended outside local-only use)
# NORIX_BUILD_SECRETS_KEY=$(openssl rand -hex 32)
```

Redirect URI to register at the IdP:

`http://127.0.0.1:8080/auth/oidc/callback`

### Configure a provider in Norix

Teams → SSO: set issuer URL, client id, client secret → **Save OIDC provider**.

Login page shows **Sign in with \<name\>** for each enabled OIDC provider.

### Local test with Dex

Minimal Dex static client (issuer `http://127.0.0.1:5556/dex`):

```yaml
issuer: http://127.0.0.1:5556/dex
storage:
  type: memory
web:
  http: 0.0.0.0:5556
staticClients:
  - id: norix-local
    redirectURIs:
      - 'http://127.0.0.1:8080/auth/oidc/callback'
    name: Norix
    secret: norix-oidc-secret
enablePasswordDB: true
staticPasswords:
  - email: "admin@example.com"
    # bcrypt hash of "password"
    hash: "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"
    username: "admin"
    userID: "08a8684b-db88-4b37-9b03-b8ac5c6a7a1d"
```

In Norix Teams SSO form:

- Issuer: `http://127.0.0.1:5556/dex`
- Client ID: `norix-local`
- Client secret: `norix-oidc-secret`

Then open the login page → **Sign in with …** → Dex → lands on `/login?sso_token=…` and the session is stored.

SAML providers can still be stored via `UpsertSSOProvider`; only OIDC has a login redirect flow.
