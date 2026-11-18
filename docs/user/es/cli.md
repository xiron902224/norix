# CLI (`norix`)

El CLI de Norix está escrito en **Rust** ([ADR-005](../../adr/ADR-005-rust-cli.md)). Usa la misma API Connect que el dashboard.

## Instalar

Desde el asset de release (`norix-cli-linux-amd64` / `arm64`), o desde fuente:

```bash
cd cli && cargo build --release
sudo install -m 755 target/release/norix /usr/local/bin/norix
```

## Inicio rápido

```bash
export NORIX_API=https://cp.example.com:8080   # sin sufijo /api
norix login --token "$SESSION_TOKEN"
norix health
norix projects list --output json
norix nodes
norix deploy --project "$PROJECT_ID" --name web --image nginx:alpine --port 80
norix logs --workload-id "$WORKLOAD_ID" -f
norix rollback --workload-id "$WORKLOAD_ID"   # release anterior
```

Completions:

```bash
norix completions bash > /etc/bash_completion.d/norix
```

## Seguridad

- El token en `~/.norix/token` queda con modo `0600`.
- `norix env` revela secretos y queda auditado (owner/admin).
- En producción usa HTTPS; HTTP en loopback vale para labs locales.
