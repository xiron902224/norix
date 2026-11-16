# Solución de problemas

| Síntoma | Qué hacer |
|---|---|
| Deploy pending / unschedulable | Docker en un worker + agent online |
| Agent offline | `norix-agent check`; URL del CP; firewall |
| Versiones de agent distintas | **Update agent** o `norix-agent update` |
| Badge de versión vacío (Vite local) | Proxy `/status` (ya en tip actual) |

```bash
norix-control-plane check
curl -sf http://127.0.0.1:8080/status | jq .
NORIX_CONTROL_PLANE_URL=https://TU_CP norix-agent check
```

Issues: https://github.com/xiron902224/norix/issues · Seguridad: [`SECURITY.md`](../../../SECURITY.md)
