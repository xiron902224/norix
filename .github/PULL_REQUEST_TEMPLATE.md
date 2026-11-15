## Summary
<!-- 1–3 bullets: what changed and why (not a file list). -->
-

## Issue
<!-- Required: link the GitHub issue. Use Fixes #N to auto-close. -->
- Fixes #
- Refs #

## Type
<!-- Check one -->
- [ ] `fix` — bug fix
- [ ] `feat` — new feature / enhancement
- [ ] `docs` — documentation only
- [ ] `chore` — tooling, CI, deps, release
- [ ] `refactor` — no intended behavior change

## Test plan
<!-- Concrete steps a reviewer can run. Prefer copy-paste commands. -->
- [ ]
- [ ]

### Cómo probarlo
<!-- Required for behavior changes (workspace rule). Skip only for pure docs/chore with no runtime impact. -->
1. **Preconditions:**
2. **Steps:**
3. **Expected:**
4. **Optional check:** `curl` / `docker` / logs

## Risk / rollback
<!-- Who/what can break? How to revert? -->
- Risk:
- Rollback:

## Checklist
- [ ] Branched from `main`; PR targets **`main`**
- [ ] Title follows: `type(#issue): short imperative summary`
- [ ] No secrets (`.env`, tokens, keys) in the diff
- [ ] CI green (protos → DB/RLS + web + agent → control-plane)
- [ ] Docs / CHANGELOG updated if user-facing
