# Norix — contribution backlog

Canonical tracker: **[GitHub Issues](https://github.com/xiron902224/norix/issues)** (snapshot 2026-11-15).  
Prefer linking/closing GitHub issues over editing this file day-to-day.

Vision / stages: [`VISION.md`](VISION.md) · Sprint detail: [`docs/ESTADO_Y_PLAN_V1.md`](docs/ESTADO_Y_PLAN_V1.md)  
**Cómo contribuir (ES):** [`CONTRIBUTING.es.md`](CONTRIBUTING.es.md) · **EN:** [`CONTRIBUTING.md`](CONTRIBUTING.md)

**Gitflow:** take an issue → branch from **`main`** → PR to **`main`**.

---

## Project state (honest)

| | |
|---|---|
| Version | `1.0.0-beta.5` |
| Default branch | `main` (integration tip) |
| `develop` | Retired for new work |
| Stage 0 (trust/safety) | **Still open** — P0 security/docs |
| Recent gap (labs) | Agents `online` without Docker → DeployFromGit fails |

---

## Snapshot counts

| Priority | Role |
|---|---|
| **P0** | Trust, safety, first impressions |
| **P1** | Table stakes for rc / production walkthrough |
| **P2** | Confidence, UX, CI depth |
| **P3** | Differentiation + ecosystem |

Plus **good first issue** labels for docs and small defaults.

---

## New issues (2026-11-15 triage)

| # | Title | Why |
|---|---|---|
| [#59](https://github.com/xiron902224/norix/issues/59) | DeployFromGit opaque `internal` on unschedulable | Same mapping as `Deploy` |
| [#60](https://github.com/xiron902224/norix/issues/60) | Clear error when no node has Docker/containerd | Lab failure mode |
| [#62](https://github.com/xiron902224/norix/issues/62) | Publish `v1.0.0-beta.4` GitHub Release | Pins/assets |
| [#63](https://github.com/xiron902224/norix/issues/63) | install-agent warn if no runtime socket | Early feedback |
| [#64](https://github.com/xiron902224/norix/issues/64) | Retire develop / main-only gitflow | Process |
| [#65](https://github.com/xiron902224/norix/issues/65) | Dashboard i18n (Spanish) | After docs.es |
| [#66](https://github.com/xiron902224/norix/issues/66) | Docs: Docker required on workers for git builds | Lab runbooks |
| [#67](https://github.com/xiron902224/norix/issues/67) | Align CI check names with branch protection | Unblock merges |

---

## Stage 0 — Trust & safety (P0 / early P1)

| # | Title |
|---|---|
| [#3](https://github.com/xiron902224/norix/issues/3) | Verify GitHub webhook signatures |
| [#4](https://github.com/xiron902224/norix/issues/4) | Block SSRF in Git-based builds |
| [#5](https://github.com/xiron902224/norix/issues/5) | Verify integrity of the installer and bare binary |
| [#6](https://github.com/xiron902224/norix/issues/6) | Require TLS (or localhost) before first-admin bootstrap |
| [#7](https://github.com/xiron902224/norix/issues/7) | Fail closed when the secrets key is missing |
| [#8](https://github.com/xiron902224/norix/issues/8) | Encrypt workload environment variables at rest |
| [#9](https://github.com/xiron902224/norix/issues/9) | Safe CORS defaults in the installer |
| [#10](https://github.com/xiron902224/norix/issues/10) | Harden and document Docker-socket exposure |
| [#11](https://github.com/xiron902224/norix/issues/11) | Always require the preview-webhook secret |
| [#12](https://github.com/xiron902224/norix/issues/12) | Add vulnerability scanning to CI |
| [#30](https://github.com/xiron902224/norix/issues/30) | Refresh the status doc and publish a maturity matrix |
| [#33](https://github.com/xiron902224/norix/issues/33) | Repository metadata and first impressions |

**Suggested next P0/P1 slice:** #9 · #7 · #12 · #3 · #59 · #60 · #62.

---

## Stage 1 — Table stakes (→ rc.1)

[#14](https://github.com/xiron902224/norix/issues/14) rollback · [#15](https://github.com/xiron902224/norix/issues/15) rolling · [#16](https://github.com/xiron902224/norix/issues/16) volumes · [#17](https://github.com/xiron902224/norix/issues/17) probes · [#18](https://github.com/xiron902224/norix/issues/18) buildpacks · [#20](https://github.com/xiron902224/norix/issues/20) managed DBs · [#22](https://github.com/xiron902224/norix/issues/22) PAT · [#23](https://github.com/xiron902224/norix/issues/23) env UX · [#26](https://github.com/xiron902224/norix/issues/26) HTTPS OOTB · [#28](https://github.com/xiron902224/norix/issues/28) CLI · [#29](https://github.com/xiron902224/norix/issues/29) SDKs · [#31](https://github.com/xiron902224/norix/issues/31) TLS guide.

---

## Stage 2 — Production confidence (→ GA)

[#37](https://github.com/xiron902224/norix/issues/37) multi-node E2E · [#38](https://github.com/xiron902224/norix/issues/38) upgrades · [#39](https://github.com/xiron902224/norix/issues/39)–[#42](https://github.com/xiron902224/norix/issues/42) coverage/load/chaos/Playwright · [#43](https://github.com/xiron902224/norix/issues/43)/[#45](https://github.com/xiron902224/norix/issues/45) footprint/limits · [#44](https://github.com/xiron902224/norix/issues/44) faster reconcile · plus onboarding/previews/notifications/doctor/examples.

---

## Stage 3–4 — Differentiation & ecosystem

[#46](https://github.com/xiron902224/norix/issues/46)–[#52](https://github.com/xiron902224/norix/issues/52) runtimes/mesh/HA · [#53](https://github.com/xiron902224/norix/issues/53)–[#57](https://github.com/xiron902224/norix/issues/57) MCP/catalog/SAML/quotas/Terraform · [#65](https://github.com/xiron902224/norix/issues/65) UI i18n ES.

---

## How to pick work

1. **P0 / security** if you know Go control-plane or installer shells.
2. **good first issue** for docs and small defaults.
3. **P1 product gaps** (rollback, volumes, buildpacks) for user-visible impact.

Comment on the GitHub issue before large work; open PRs against **`main`**.
