# Ops y backups

Backups, restore, alertas y exports. Haz backup **antes** de actualizar ([guía](../../UPDATE_AND_UNINSTALL.md)). Verifica `/healthz` y agents online después.

## Retención de logs

El líder del control plane poda `workload_logs`, `build_logs` y `log_segments` cada ~20 s, por edad, por número de líneas y por bytes. La ficha del workload muestra esos defaults encima de los paneles de logs. Un deploy de imagen solo tiene logs de runtime; los de build aparecen tras un build desde git. Reinicia el proceso después de cambiar las variables.

| Variable | Default | Significado |
|---|---|---|
| `NORIX_LOG_RETENTION_DAYS` | `14` | Borra filas más viejas. `0` desactiva la poda por edad. |
| `NORIX_WORKLOAD_LOG_MAX_LINES` | `500` | Líneas más nuevas que se conservan por workload. |
| `NORIX_BUILD_LOG_MAX_LINES` | `800` | Líneas más nuevas por build. |
| `NORIX_LOG_SEGMENT_MAX_LINES` | `5000` | Segmentos más nuevos por organización. |
| `NORIX_LOG_RETENTION_BYTES` | `33554432` (32 MiB) | Bytes más nuevos por workload. `0` desactiva el tope de bytes. |
| `NORIX_BUILD_LOG_RETENTION_BYTES` | `33554432` | Bytes más nuevos por build. |
| `NORIX_LOG_SEGMENT_RETENTION_BYTES` | `67108864` (64 MiB) | Bytes más nuevos por organización. |

Un valor no numérico vuelve al default. El tope de líneas se recorta a su rango. Un tope de bytes fuera de 0–10 GiB vuelve al default. El tope de líneas también se aplica cuando el agente reporta logs.

## Restaurar

Restaurar sustituye el esquema `public` de la base del control plane. Un `pg_dump` plano solo trae `CREATE`, así que cargarlo encima de una base que ya tiene `alert_events` falla con `relation already exists` (psql exit 3).

El control plane vuelca con `--clean --if-exists` y, antes de `psql -f`, corta las otras sesiones y hace `DROP SCHEMA public CASCADE`. Reinicia el control plane después de restaurar. Los dumps viejos, sin `DROP`, se restauran igual.
