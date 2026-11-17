# Historial de releases y rollback

Cada **Deploy** / **Start** / deploy de réplicas Compose guarda un **release** inmutable: imagen (y digest si existe), claves de env enmascaradas, puerto del contenedor, límites CPU/memoria y generación.

## Panel

1. Abre un workload → pestaña **Releases**.
2. El historial va de más nuevo a más viejo. La primera fila es **latest**.
3. Elige un release anterior → **Roll back** → confirma. Norix restaura ese snapshot y vuelve a desplegar.

Los valores de env en la lista siguen enmascarados (`********`). El rollback restaura los secretos cifrados que se guardaron con ese release.

## API

- `ListWorkloadReleases` — historial de un workload (`limit` opcional).
- `RollbackWorkload` — `workload_id` + `release_id` (rol mutator).

## Variables de ops

| Variable | Por defecto | Significado |
|---|---|---|
| `NORIX_RELEASE_RETENTION` | `10` | Máximo de releases por workload (1–100). Se podan los más viejos al grabar. |
| `NORIX_AUTO_ROLLBACK` | off | Con `1`/`true`, un status `failed` tras apply dispara rollback al snapshot **deploy** anterior (se omite si el último ya es rollback). |

El auto-rollback es best-effort y conservador (no entra en bucle tras un rollback auto/manual).

## Consejos

- Prefiere imágenes con digest (`@sha256:…`) para fijar bits exactos.
- Mantén retención acorde a tu ritmo de deploys.
- Combínalo con [#15](https://github.com/xiron902224/norix/issues/15) (rolling) cuando esté listo.
