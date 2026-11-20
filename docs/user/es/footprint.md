# Huella de recursos

Tamaños medidos de los binarios Norix y RSS en reposo (#43). Vuelve a medir:

```bash
./scripts/measure-footprint.sh
./scripts/measure-footprint.sh --check   # presupuestos de CI
```

## Números publicados (Ubuntu amd64, release)

| Binario | Tamaño (MiB) | RSS en reposo (MiB) | Notas |
|---|---:|---:|---|
| `norix-control-plane` | ~30–40 | ~40–70 | Necesita Postgres; RSS tras ~2 s |
| `norix-agent` | ~4–8 | ~10–30 | Mientras reintenta un control plane inválido |

Los valores exactos cambian con la toolchain. CI aplica techos blandos (`NORIX_FOOTPRINT_*_MIB`). Para límites de escala: [Scale limits](../scale-limits.md) (EN).
