# CLI (`norix`)

The Norix CLI is written in **Rust** ([ADR-005](../adr/ADR-005-rust-cli.md)). It calls the same Connect API as the dashboard.

## Install

From a release asset (`norix-cli-linux-amd64` / `arm64`), or build from source:

```bash
cd cli && cargo build --release
sudo install -m 755 target/release/norix /usr/local/bin/norix
```

## Quick start

```bash
export NORIX_API=https://cp.example.com:8080   # no /api suffix
norix login --token "$SESSION_TOKEN"
norix health
norix projects list --output json
norix nodes
norix deploy --project "$PROJECT_ID" --name web --image nginx:alpine --port 80
norix logs --workload-id "$WORKLOAD_ID" -f
norix rollback --workload-id "$WORKLOAD_ID"   # previous release
```

Shell completions:

```bash
norix completions bash > /etc/bash_completion.d/norix
```

## Security notes

- Tokens in `~/.norix/token` use mode `0600`.
- `norix env` reveals secrets and is audited (owner/admin).
- Prefer HTTPS in production; loopback HTTP is fine for local labs.
