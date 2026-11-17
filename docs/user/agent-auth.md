# Agent authentication

## Default: node bearer token

On enroll, the control plane returns a **node token**. The agent stores it in `node-state.json` and sends `Authorization: Bearer …` on heartbeat and reports.

### Rotate (UI / API)

**Nodes** → select agent → **Rotate token**. The next successful heartbeat returns a new token; the agent overwrites local state. The previous token works for ~15 minutes.

### Revoke

**Revoke token** invalidates the bearer immediately. Re-enroll with a fresh **Join token**.

## Optional mTLS

1. Issue a client certificate for the node (external CA).
2. Compute SHA-256 of the DER cert (`openssl x509 -in node.crt -outform DER | sha256sum`).
3. Nodes → paste fingerprint → **Save client cert**.
4. Control plane HTTPS: `NORIX_TLS_CERT`, `NORIX_TLS_KEY`, `NORIX_AGENT_MTLS_CA` (CA PEM that signed node certs).
5. Agent: `NORIX_AGENT_CLIENT_CERT`, `NORIX_AGENT_CLIENT_KEY`, optional `NORIX_AGENT_CA_CERT`.
6. Strict mode: `NORIX_REQUIRE_AGENT_MTLS=1`.

Design: [ADR-004](../adr/ADR-004-agent-auth.md). Español: [es/agent-auth.md](./es/agent-auth.md).
