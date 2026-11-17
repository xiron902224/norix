# Release history & rollback

Every successful **Deploy** / **Start** / compose replica deploy records an immutable **release** snapshot: image (and digest when present), masked env keys, container port, CPU/memory limits, and generation.

## Dashboard

1. Open a workload → **Releases** tab.
2. History is newest-first. The top row is **latest**.
3. Pick an older release → **Roll back** → confirm. Norix restores that snapshot and redeploys.

Env values in the list stay masked (`********`). Rollback restores the encrypted secrets that were stored with that release.

## API

- `ListWorkloadReleases` — history for a workload (`limit` optional).
- `RollbackWorkload` — `workload_id` + `release_id` (mutator role).

## Ops knobs

| Variable | Default | Meaning |
|---|---|---|
| `NORIX_RELEASE_RETENTION` | `10` | Max releases kept per workload (1–100). Older rows are pruned after each record. |
| `NORIX_AUTO_ROLLBACK` | off | When `1`/`true`, a reported `failed` status after apply triggers rollback to the previous **deploy** snapshot (skips if latest is already a rollback). |

Auto-rollback is best-effort and intentionally conservative (no loop after an auto/manual rollback snapshot).

## Tips

- Prefer image tags with digests (`@sha256:…`) so rollbacks pin the exact bits.
- Keep retention high enough for your deploy cadence; very low values leave little history.
- Combine with [#15](https://github.com/xiron902224/norix/issues/15) rolling updates once available for safer forward deploys.
