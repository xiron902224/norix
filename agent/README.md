# Norix agent (v1.0)

Rust node agent: enroll, heartbeat inventory, container reconcile, git builds, edge (Caddy), mesh (WireGuard/nft/DNS), Wasm staging, and micro-VM boot intents.

## Build

```bash
cargo build --release
```

Binary: `target/release/norix-agent`

## Run

```bash
cp .env.example .env   # if present
export NORIX_JOIN_TOKEN=...   # from dashboard Nodes → join token
export NORIX_CONTROL_PLANE_URL=http://127.0.0.1:8080
./target/release/norix-agent
# or: cargo run --release
```

Requires `docker` or `nerdctl` on PATH for container Deploy/Build. Without a runtime the agent still heartbeats (degraded).

### Optional env

| Variable | Purpose |
|----------|---------|
| `NORIX_APPLY_WIREGUARD` | On **control plane**: default **on** (apply). Set `0`/`false`/`dry-run` for configs-only. Agents still skip live apply if `wg-quick`/`nft` missing; failures roll back (#48) |
| `NORIX_MICROVM_KERNEL` | Kernel path for Cloud Hypervisor |
| `NORIX_MICROVM_ROOTFS` | Rootfs disk path for Cloud Hypervisor |
| `NORIX_CLOUD_HYPERVISOR` | Hypervisor binary (default `cloud-hypervisor`) |
| `NORIX_AGENT_LOG_LEVEL` | tracing filter (default `info`) |

## What it reconciles

1. **Containers / static-site / compose images** — pull/run/rm with labels `norix.*`, `--memory`, `--cpu-shares`
2. **Git builds** — clone → Dockerfile/compose → tag → `ReportBuild`
3. **Edge** — Caddy admin `/load` + rollback
4. **Mesh** — write `wg-norix.conf`, `norix.nft`, `dns-hosts`, certs; apply when `plan.apply`
5. **Wasm** — `DesiredWasmPlan` from heartbeat → stage `.wasm` / OCI extract → `functions.json` → `ReportWasm` (smoke with `wasmtime` if installed)
6. **Micro-VM** — KVM check + boot-intent JSON; launches CH when kernel/rootfs env set

Heartbeat sends `running_workload_ids` from containers labeled `norix.workload_id`.

## Installer

```bash
./scripts/install-agent.sh --control-plane http://127.0.0.1:8080 --token "$JOIN_TOKEN" --from-source
```

## Check

```bash
./target/release/norix-agent --check
```

## Honest v1 limits

- Full WASI HTTP worker still needs a dedicated wasmtime/WASI host process beyond staging + optional smoke.
- Micro-VM cold boot requires real KVM + kernel/rootfs artifacts on the node.
- Mesh apply defaults on; opt out with `NORIX_APPLY_WIREGUARD=0` (see `docs/user/network.md`, #48).
