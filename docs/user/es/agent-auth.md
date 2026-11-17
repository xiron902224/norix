# Autenticación del agent

## Por defecto: bearer del nodo

Tras el enroll, el CP devuelve un **node token**. El agent lo guarda en `node-state.json` y lo envía como `Authorization: Bearer …`.

### Rotar

**Nodes** → agent → **Rotate token**. El siguiente heartbeat entrega el token nuevo; el anterior vale ~15 minutos.

### Revocar

**Revoke token** invalida al momento. Hay que re-enroll con un **Join token** nuevo.

## mTLS opcional

1. Certificado cliente del nodo (CA externa).
2. SHA-256 del DER: `openssl x509 -in node.crt -outform DER | sha256sum`.
3. Nodes → pegar fingerprint → **Save client cert**.
4. CP en HTTPS: `NORIX_TLS_CERT`, `NORIX_TLS_KEY`, `NORIX_AGENT_MTLS_CA`.
5. Agent: `NORIX_AGENT_CLIENT_CERT`, `NORIX_AGENT_CLIENT_KEY`.
6. Estricto: `NORIX_REQUIRE_AGENT_MTLS=1`.

Diseño: [ADR-004](../../adr/ADR-004-agent-auth.md).
