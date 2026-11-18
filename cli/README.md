# Norix CLI (`norix`)

**Language: Rust** (see [ADR-005](../docs/adr/ADR-005-rust-cli.md)). Same stack family as the node agent.

Talks to the control plane over Connect JSON (`/api/v1.*Service/...`).

## Build

```bash
cd cli
cargo build --release
./target/release/norix --help
```

## Auth

```bash
export NORIX_API=http://127.0.0.1:8080
norix login --token "$SESSION_TOKEN"   # stores ~/.norix/token (0600)
# or: export NORIX_TOKEN=...
```

## Commands (#28)

| Command | Purpose |
|---|---|
| `health` | `GET /healthz` |
| `login` | Save bearer token |
| `projects list` | List projects |
| `workloads list --project ID` | List workloads |
| `deploy --project … --name … --image …` | Create + deploy image workload |
| `deploy --workload-id …` | Redeploy existing |
| `logs --workload-id … [-f]` | Tail / follow runtime logs |
| `env --workload-id …` | Reveal env (audited) |
| `domains --project …` | List domain routes |
| `nodes` | List nodes |
| `releases --workload-id …` | Release history |
| `rollback --workload-id … [--release-id …]` | Rollback |
| `completions bash\|zsh\|fish\|…` | Shell completions |

Global: `--api`, `--token`, `--output json|text`.

## Tests

```bash
cd cli && cargo test
```

Uses `httpmock` — no live control plane required.
