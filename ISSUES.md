# Norix — contribution backlog

Canonical tracker: **[GitHub Issues](https://github.com/xiron902224/norix/issues)** (55 open as of 2026-11-15).  
This file is a **snapshot + triage** so newcomers see priorities without scrolling the board. Prefer linking/closing GitHub issues over editing this file for day-to-day work.

Vision / stages: [`VISION.md`](VISION.md) · Sprint detail: [`docs/ESTADO_Y_PLAN_V1.md`](docs/ESTADO_Y_PLAN_V1.md)

---

## Snapshot (2026-11-15)

| Priority | Count | Role |
|---|---:|---|
| **P0** | 10 | Trust, safety, first impressions — block confident public beta ops |
| **P1** | 14 | Table stakes for rc / production walkthrough |
| **P2** | 19 | Confidence, UX polish, CI depth |
| **P3** | 12 | Differentiation + ecosystem (1.x → 2.0) |
| **good first issue** | 17 | Docs, small security defaults, coverage |

**Stages** (labels `stage/0`…`stage/4`): 11 / 14 / 9 / 7 / 5 — plus 9 unstaged (mostly docs/CI P2).

### What beta.3 does *not* close

Stage 0 (trust & safety) is **still open**. Beta.3 ships installer/agent supervise + docs refresh; the P0 security issues below remain the next focus before calling Stage 0 done.

---

## Stage 0 — Trust & safety (P0 / early P1)

| # | Title | Notes |
|---|---|---|
| [#3](https://github.com/xiron902224/norix/issues/3) | Verify GitHub webhook signatures | HMAC |
| [#4](https://github.com/xiron902224/norix/issues/4) | Block SSRF in Git-based builds | |
| [#5](https://github.com/xiron902224/norix/issues/5) | Verify integrity of the installer and bare binary | Partial: release SHA256 in get-norix / install-agent |
| [#6](https://github.com/xiron902224/norix/issues/6) | Require TLS (or localhost) before first-admin bootstrap | |
| [#7](https://github.com/xiron902224/norix/issues/7) | Fail closed when the secrets key is missing | GFI |
| [#8](https://github.com/xiron902224/norix/issues/8) | Encrypt workload environment variables at rest | P1 |
| [#9](https://github.com/xiron902224/norix/issues/9) | Safe CORS defaults in the installer | GFI · P0 |
| [#10](https://github.com/xiron902224/norix/issues/10) | Harden and document Docker-socket exposure | |
| [#11](https://github.com/xiron902224/norix/issues/11) | Always require the preview-webhook secret | GFI · P1 |
| [#12](https://github.com/xiron902224/norix/issues/12) | Add vulnerability scanning to CI | GFI |
| [#30](https://github.com/xiron902224/norix/issues/30) | Refresh status doc + maturity matrix | GFI · addressed in VISION / this release |
| [#33](https://github.com/xiron902224/norix/issues/33) | Repository metadata and first impressions | GFI |

**Suggested next P0 slice:** #9 CORS · #7 secrets fail-closed · #12 vuln scan · #3 webhook HMAC.

---

## Stage 1 — Table stakes (→ rc.1)

Rollbacks [#14], rolling updates [#15], volumes [#16], probes [#17], buildpacks [#18], managed DBs for real [#20], env/secret UX [#23], HTTPS OOTB [#26], PAT [#22], CLI [#28], typed SDKs [#29], TLS guide [#31], onboarding [#19], previews E2E [#21].

---

## Stage 2 — Production confidence (→ GA)

Multi-node E2E [#37], upgrade tests [#38], agent coverage [#39], load [#40], chaos [#41], Playwright [#42], footprint [#43], scale limits [#45], faster reconcile [#44], log retention [#25], notifications [#27], doctor/preflight [#34], examples [#35], troubleshooting [#32], build progress UX [#36], agent auth beyond bearer [#13], parallel builds [#24].

---

## Stage 3 — Differentiation (1.x)

Micro-VM GA [#46], Wasm GA [#47], WireGuard default [#48], autoscaler GA [#49], Sandboxes API [#50], Firecracker [#51], Postgres HA runbook [#52].

---

## Stage 4 — Ecosystem

MCP [#53], app catalog [#54], SAML [#55], quotas [#56], Terraform [#57].

---

## How to pick work

1. **Security / P0** if you know Go control-plane or installer shells.
2. **good first issue** for docs and small defaults (`#9`, `#7`, `#31`, `#32`, `#35`).
3. **P1 product gaps** (`#14` rollback, `#16` volumes, `#18` buildpacks) if you want user-visible impact.

Comment on the GitHub issue before large work; open PRs against `develop`.
