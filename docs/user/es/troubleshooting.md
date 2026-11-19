# Solución de problemas

Guía completa (EN): [`../troubleshooting.md`](../troubleshooting.md).

| Síntoma | Qué hacer |
|---|---|
| Deploy / `no online node with Docker or containerd` | Docker en un worker + agent online |
| Agent offline | `norix-agent check`; URL del CP; firewall |
| Installer / CP no arranca | `NORIX_BUILD_SECRETS_KEY` (64 hex) o docs de bare/docker |
| Env muestra `********` | Normal (#8): cifrado en reposo; el agent recibe el valor al aplicar |
| Webhook 401 | Secret de GitHub = token del path; firma HMAC |
| TLS / login loop | `NORIX_PUBLIC_URL` / `NORIX_WEB_ORIGIN` en HTTPS ([TLS prod](./production-tls.md)) |
| Micro-VM / KVM | Experimental; hace falta `/dev/kvm` en el nodo |
| Versiones de agent distintas | **Update agent** o `norix-agent update` |
| Badge de versión vacío (Vite local) | Proxy `/status` |
| Antes de instalar | `norix doctor` o `scripts/install.sh preflight` (#34) |

```bash
norix-control-plane check
curl -sf http://127.0.0.1:8080/status | jq .
NORIX_CONTROL_PLANE_URL=https://TU_CP norix-agent check
```

Issues: https://github.com/xiron902224/norix/issues · Seguridad: [`SECURITY.md`](../../../SECURITY.md)
