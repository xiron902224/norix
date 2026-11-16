# Documentation map

Where to read / write docs in this repo. Collaborators: start at [`CONTRIBUTING.md`](../CONTRIBUTING.md) / [`.es.md`](../CONTRIBUTING.es.md).

| Audience | Start here |
|---|---|
| **Platform users** (dashboard) | **[`user/`](./user/README.md)** · [Español](./user/es/README.md) — official product guide |
| **Operators** (install on a VPS) | [`README.md`](../README.md) · [`README.es.md`](../README.es.md) · [`runbooks/BARE_INSTALL.md`](./runbooks/BARE_INSTALL.md) |
| **Collaborators / developers** | [`CONTRIBUTING.md`](../CONTRIBUTING.md) · [`CONTRIBUTING.es.md`](../CONTRIBUTING.es.md) — local: Postgres + CP `:8080` + Vite `:5173` (+ agent) |
| **Product status / roadmap** | [`VISION.md`](../VISION.md) · [`ESTADO_Y_PLAN_V1.md`](./ESTADO_Y_PLAN_V1.md) · [`ISSUES.md`](../ISSUES.md) |
| **Multi-node ops** | [`runbooks/BETA1_3NODE_INSTALL.md`](./runbooks/BETA1_3NODE_INSTALL.md) |
| **Security** | [`SECURITY.md`](../SECURITY.md) · [`threat-model-s19.md`](./threat-model-s19.md) |
| **Upgrade / uninstall** | [`UPDATE_AND_UNINSTALL.md`](./UPDATE_AND_UNINSTALL.md) · [`UPGRADE.md`](./UPGRADE.md) |
| **Component READMEs** | [`agent/`](../agent/README.md) · [`control-plane/`](../control-plane/README.md) · [`web/`](../web/README.md) |
| **Sprint history** | `SPRINT_*_CIERRE.md` (archive; prefer VISION / ESTADO) |
| **ADRs** | [`adr/`](./adr/) |

## Language

- English is canonical for code comments and most `docs/`.
- Spanish entry points: `README.es.md`, `CONTRIBUTING.es.md`.
- Runbooks may be Spanish when they target the local ops audience.

## Writing rules

- User-facing behavior change → update the relevant runbook + short **Cómo probarlo** in the PR template.
- Do not duplicate long install guides in CONTRIBUTING; link to README / BARE_INSTALL.
- Prefer linking GitHub issues over copying backlog into sprint closure docs.
- Contributors on the README use [contrib.rocks](https://contrib.rocks) (auto from merged PRs) plus a short maintainer table.
