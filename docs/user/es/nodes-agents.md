# Nodos y agents

## Unir un nodo

1. **Regions & nodes** → join token  
2. En el worker:

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/v1.0.0-beta.4/scripts/install-agent.sh \
  | bash -s -- --version v1.0.0-beta.4 \
      --control-plane 'https://TU_CP' \
      --token 'JOIN_TOKEN'
```

Sin systemd: `--supervise none`.

## Controles

Cordon · Drain · role/zone.

## Actualizar

- UI → **Update agent**  
- `norix-agent check` (sin token)  
- `norix-agent update --version v1.0.0-beta.4`  

Ver [`UPDATE_AND_UNINSTALL.md`](../../UPDATE_AND_UNINSTALL.md).
