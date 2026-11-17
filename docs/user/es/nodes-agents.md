# Nodos y agents

## Unir un nodo

1. **Regions & nodes** → join token  
2. En el worker:

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/v1.0.0-beta.5/scripts/install-agent.sh \
  | bash -s -- --version v1.0.0-beta.5 \
      --control-plane 'https://TU_CP' \
      --token 'JOIN_TOKEN'
```

Sin systemd: `--supervise none`.

Los installs desde release verifican **SHA256SUMS** y **cosign** si hay `.cosign.bundle` (`--skip-cosign` / `NORIX_SKIP_COSIGN=1` solo en labs).

Si no hay socket de Docker/containerd, `install-agent.sh` imprime un **WARNING** (el enroll sigue); sin runtime este nodo no recibe builds/deploys.

Si el socket **sí** está, el agent lo monta — es potente (≈ root del host con Docker rootful). En Compose de producción el socket no va al control plane; ver [docker-socket](./docker-socket.md).

## Controles

Cordon · Drain · role/zone.

## Actualizar

- UI → **Update agent**  
- `norix-agent check` (sin token)  
- `norix-agent update --version v1.0.0-beta.5`  

Ver [`UPDATE_AND_UNINSTALL.md`](../../UPDATE_AND_UNINSTALL.md).
