# Security Policy

## Reporting

Report vulnerabilities **privately** to the maintainers. Do not open public issues for active exploits.

## Supported versions

| Version | Supported |
|---|---|
| 1.0.0-beta.x | Best effort (current: beta.5) |
| 1.0.x (GA) | Yes, once released |
| < 1.0 / alpha | Best effort |

## Installer / release integrity

Bare install and agent install verify **SHA256SUMS** before installing release binaries. When a Sigstore **cosign** bundle is published on the GitHub Release, verification is required (fail-closed) unless you explicitly set `NORIX_SKIP_COSIGN=1` / `--skip-cosign` (labs only).

Manual download → verify → run: see [README](README.md#verify-release-binaries-download--verify--run) / [README.es.md](README.es.md#verificar-binarios-del-release-bajar--verificar--ejecutar).

## Secrets

Never commit `.env`, join tokens, or `NORIX_BUILD_SECRETS_KEY`. Rotate join tokens after use.
