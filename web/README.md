# Web frontend

The Norix frontend is the user-facing shell for bootstrap, authentication and a basic dashboard experience.

## Local run

1. Create the environment file:

```bash
cp .env.example .env
```

2. Install dependencies:

```bash
npm install
```

3. Start the dev server:

```bash
npm run dev -- --host 0.0.0.0
```

4. Open:

```text
http://127.0.0.1:5173
```

## Environment variables

```dotenv
VITE_NORIX_API_URL=http://127.0.0.1:8080/api
```

## Build and preview

```bash
npm run build
npm run preview -- --host 0.0.0.0
```

## Architecture

The frontend follows a lightweight Atomic Design split:

- shared/ui: buttons, inputs and generic UI atoms
- features/auth/components: bootstrap/login forms
- features/auth/services: session and API calls
- App.tsx: state orchestration for bootstrap/login/logout

## Production embed

Production builds are copied into the control plane and served from the Go binary:

```bash
# from repo root
make embed-web
make control-plane
```

Playwright E2E (requires a running control plane):

```bash
# Smoke + lifecycle (bootstrap → project → image → domain → logs → delete).
# Deploy-to-running needs an agent — set NORIX_E2E_FULL=1 for that path.
NORIX_E2E=1 npm run test:e2e
NORIX_E2E=1 NORIX_E2E_FULL=1 npm run test:e2e
```

Specs live under `e2e/` (`smoke.spec.ts`, `lifecycle.spec.ts`). Gate is `NORIX_E2E`; see `playwright.config.ts`.

## Notes

- Auth bootstrap/login plus an authenticated projects/workloads dashboard.
- Deploy remains disabled until a runtime agent is enrolled (S3/S4).
- Same-origin `/api` is the default API URL for the embedded UI.
- Load tests (k6): [`docs/user/load-tests.md`](../docs/user/load-tests.md).
