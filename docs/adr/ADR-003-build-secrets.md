# ADR-003: Build secrets encryption at rest

## Status

Accepted (S5)

## Context

Git builds may need private registry tokens or compile-time secrets (`--build-arg`).
Storing plaintext in PostgreSQL is unacceptable even for Alpha.

## Decision

- Encrypt build secrets with **AES-256-GCM**.
- Key comes from env `NORIX_BUILD_SECRETS_KEY` (32 raw bytes encoded as 64 hex chars).
- Ciphertext + nonce live in `build_secrets`; plaintext is only decrypted when a node claims a pending build.
- Secrets are optional: builds without secrets do not require the key.

## Consequences

- Operators must set/rotate the key out of band (Compose/env).
- Multi-key / per-org wrapping with **age** remains a S8 hardening candidate; AES-GCM is enough for single-node v1.
- Losing the key makes stored secrets unreadable (by design).
