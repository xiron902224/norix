# Resource footprint

Measured sizes for Norix binaries and idle RSS (#43). Re-run:

```bash
./scripts/measure-footprint.sh
./scripts/measure-footprint.sh --check   # CI budgets
```

## Published numbers (Ubuntu amd64, release builds)

| Binary | Size (MiB) | Idle RSS (MiB) | Notes |
|---|---:|---:|---|
| `norix-control-plane` | ~30–40 | ~40–70 | Needs Postgres; RSS sampled after ~2s up |
| `norix-agent` | ~4–8 | ~10–30 | Sampled while retrying a bad control-plane URL |

Exact values move with Go/Rust toolchain and linked features. CI enforces soft ceilings:

| Budget env | Default |
|---|---|
| `NORIX_FOOTPRINT_CP_BIN_MIB` | `45` |
| `NORIX_FOOTPRINT_AGENT_BIN_MIB` | `12` |
| `NORIX_FOOTPRINT_CP_RSS_MIB` | `80` |
| `NORIX_FOOTPRINT_AGENT_RSS_MIB` | `40` |

Loaded RSS/CPU depends on workload count, log retention, and build concurrency. For scale caps see [Scale limits](./scale-limits.md).
