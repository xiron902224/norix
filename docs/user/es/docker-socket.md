# Exposición del socket de Docker

## Por qué importa

`/var/run/docker.sock` es la API del Engine. Quien pueda hablarle puede crear contenedores privilegiados y montar el filesystem del host — con un daemon rootful eso es **root en el host**.

Norix necesita *algún* componente que arranque contenedores. La pregunta es **cuál** monta el socket.

## Por defecto (recomendado): el agent tiene el socket

En modo Docker (`deploy/norix/docker-compose.prod.yml`):

| Servicio | `docker.sock` | Rol |
|---|---|---|
| **control-plane** | **No** | API, UI, scheduling |
| **agent** | **Sí** | Builds + reconcile en este host |
| postgres | No | Datos |

Trade-off: el agent (y quien lo comprometa) sigue pudiendo usar la API de Docker. El **panel/API** tiene menor blast radius.

El installer escribe `NORIX_JOIN_TOKEN` / `NORIX_BOOTSTRAP_JOIN_TOKEN` en `.env`. Tras el primer admin, el control plane siembra ese join token para que el agent de Compose se enroll (reintentos con `restart: unless-stopped`).

## Opt-in: runtime embebido (el CP tiene el socket)

Camino legacy one-box: montar el socket en el control plane y `NORIX_EMBEDDED_RUNTIME=1`.

```bash
docker compose --env-file .env \
  -f deploy/norix/docker-compose.prod.yml \
  -f deploy/norix/docker-compose.embedded-runtime.yml up -d
```

Solo en un host **de confianza**. Preferir el agent por defecto si hay exposición a internet.

## Endurecimiento opcional

### Socket proxy

[Tecnativa docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy/) (u otro) delante del socket real; el agent usa `DOCKER_HOST=tcp://docker-proxy:2375` con allow-list.

Matices: un agent Norix necesita API amplia (create/build/start). El proxy **reduce** riesgo; no lo elimina. Incluso GET puede filtrar datos sensibles de contenedores.

### Docker rootless

[Modo rootless](https://docs.docker.com/engine/security/rootless/): el impacto de un compromise se acerca al usuario, no a root del host. Apunta el agent al socket rootless (`$XDG_RUNTIME_DIR/docker.sock`).

## Aviso del installer

`get-norix.sh` (modo docker) e `install-agent.sh` advierten del poder del socket y enlazan aquí. El CP bare no necesita el socket; los workers sí.

## Relacionado

- Threat model: [`docs/threat-model-s19.md`](../../threat-model-s19.md)
- Nodos: [`nodes-agents.md`](./nodes-agents.md)
- Issue [#10](https://github.com/xiron902224/norix/issues/10)
