# Updates rolling / sin downtime

Por defecto Norix usa estrategia **rolling**: cuando cambia la generación o la imagen de una réplica, el agent arranca un contenedor **surge** en un puerto host temporal, espera readiness (HTTP `GET /` con fallback TCP), cambia el puerto publicado y entonces para el contenedor viejo. El control plane refresca upstreams de dominio/edge cuando cambia `host_port`.

## Controles (pestaña Edit)

| Campo | Por defecto | Significado |
|---|---|---|
| `update_strategy` | `rolling` | `rolling` = arrancar antes de parar; `recreate` = parar y luego arrancar (downtime breve). |
| `max_surge` | `1` | Permite reservar puerto surge. `0` con `max_unavailable=0` se corrige a surge `1` en rolling. |
| `max_unavailable` | `0` | Reservado para oleadas multi-réplica; cada réplica sigue prefiriendo start-before-stop si surge &gt; 0. |

## Ops

- Prefiere imágenes que respondan pronto en el puerto del contenedor (o `/`).
- Probes configurables: [#17](https://github.com/xiron902224/norix/issues/17).
- Combina con [rollback](./releases-rollback.md) si el roll falla tras el cutover.
