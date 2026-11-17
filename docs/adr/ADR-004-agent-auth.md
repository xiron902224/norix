# ADR-004: Agent ↔ control plane authentication

## Status

Accepted (beta.6 / #13)

## Context

Agents authenticated with a long-lived opaque **node bearer** only. There was no UI/API to rotate or revoke that credential, and no optional mTLS path. Compromised `node-state.json` meant indefinite impersonation of the node.

## Decision

1. **Bearer remains the default** for enroll + heartbeat (works behind HTTP proxies and Compose networks).
2. **Rotate** (`RotateNodeToken`): admin queues rotation; the **next Heartbeat** returns `rotated_node_token` once; agent persists it; old hash stays valid for **15 minutes** (`previous_token_hash`).
3. **Revoke** (`RevokeNodeToken`): invalidate immediately; node goes offline until re-enroll with a join token.
4. **Optional mTLS**: store `client_cert_sha256` per node (`SetNodeClientCert`). With `NORIX_TLS_CERT`/`KEY` + `NORIX_AGENT_MTLS_CA`, the CP requests/verifies client certs. Agent sets `NORIX_AGENT_CLIENT_CERT`/`KEY`. `NORIX_REQUIRE_AGENT_MTLS=1` fail-closes agent RPCs without a matching fingerprint.
5. Built-in node CA issuance is **out of scope** (operators or a future CA can mint certs; fingerprint binding is enough for v1).

## Consequences

- Operators can recover from leaked agent state without wiping the node row.
- mTLS needs HTTPS on the CP (or a TLS-terminating path that forwards client certs — not implemented; prefer CP `ListenAndServeTLS`).
- Join tokens remain single-use bootstrap; node bearers are the ongoing secret.

## References

- Issue [#13](https://github.com/xiron902224/norix/issues/13)
- [Kubernetes bootstrap tokens](https://kubernetes.io/docs/reference/access-authn-authz/bootstrap-tokens/) (join ≠ long-lived auth)
- [Go `crypto/tls` ClientAuth](https://pkg.go.dev/crypto/tls#ClientAuthType)
