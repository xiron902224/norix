# Norix agent (Sprint 3–4)

Rust agent that enrolls with a one-time join token, heartbeats with reconnect backoff, and reconciles container desired state (Docker or nerdctl).

## Build

```bash
cargo build --release
```

## Run

```bash
cp .env.example .env
export NORIX_JOIN_TOKEN=...   # from dashboard Nodes page
export NORIX_CONTROL_PLANE_URL=http://127.0.0.1:8080
cargo run --release
```

Requires `docker` or `nerdctl` on PATH for Deploy/reconcile. Without a runtime the agent still heartbeats.

## Installer

```bash
./scripts/install-agent.sh --control-plane http://127.0.0.1:8080 --token "$JOIN_TOKEN" --from-source
```

## Flow

1. Enroll → save `node-state.json`
2. Heartbeat → receive `desiredWorkloads`
3. Reconcile containers (pull/run/rm) idempotently by generation label
4. `ReportObserved` → status, ports, log lines
