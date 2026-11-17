# Security Policy

## Reporting

Report vulnerabilities **privately** to the maintainers. Do not open public issues for active exploits.

## Supported versions

| Version | Supported |
|---|---|
| 1.0.0-beta.x | Best effort (current: beta.5) |
| 1.0.x (GA) | Yes, once released |
| < 1.0 / alpha | Best effort |

## Workload environment variables

`workloads.env_vars` are stored as an AES-256-GCM envelope (`NORIX_BUILD_SECRETS_KEY`). List/get API responses mask values (`********`). Plaintext is decrypted only when building desired state for agents/local runtime. Legacy plaintext rows are re-encrypted at control-plane startup.

## First-admin bootstrap

`CreateInitialAdmin` requires a **loopback** client, **HTTPS** (trusted loopback proxy + `X-Forwarded-Proto`), or explicit `NORIX_ALLOW_INSECURE_BOOTSTRAP=1`. Docker one-click sets the opt-in for published ports; remove it after TLS. See [getting started](docs/user/getting-started.md) / [ES](docs/user/es/getting-started.md).

## Installer / release integrity

Bare install and agent install verify **SHA256SUMS** before installing release binaries. When a Sigstore **cosign** bundle is published on the GitHub Release, verification is required (fail-closed) unless you explicitly set `NORIX_SKIP_COSIGN=1` / `--skip-cosign` (labs only).

Manual download → verify → run: see [README](README.md#verify-release-binaries-download--verify--run) / [README.es.md](README.es.md#verificar-binarios-del-release-bajar--verificar--ejecutar).

## Secrets

Never commit `.env`, join tokens, or `NORIX_BUILD_SECRETS_KEY`. Rotate join tokens after use.
