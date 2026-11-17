# Bases de datos

**Databases** gestiona instancias de **bases de datos de aplicación** (Postgres, MySQL, Redis) que Norix ejecuta como contenedores en un worker.

## Crear

1. Abre **Databases** → **New database**
2. Elige proyecto + motor + nombre
3. El estado empieza en `provisioning` hasta que el agent crea el contenedor
4. Cuando está `ready`, copia el **connection URI** (la contraseña aparece como `******` en la UI)

Las credenciales van cifradas en reposo (`NORIX_BUILD_SECRETS_KEY`). Los datos viven en un **volumen Docker con nombre** en el nodo asignado — no se replican entre nodos.

## Motores

| Motor | Imagen | Puerto |
|---|---|---|
| PostgreSQL | `postgres:16-alpine` | puerto host → 5432 |
| MySQL | `mysql:8.4` | → 3306 |
| Redis | `redis:7-alpine` | → 6379 |

MongoDB está en el catálogo pero **aún no** se provisiona.

## Backups

Usa **Backup** sobre una instancia `ready`. El control plane hace dump con `docker exec` (`pg_dump` / `mysqldump` / Redis SAVE).

## Ops

1. Los datos son locales al nodo.
2. Pega el URI en las env vars del workload y redespliega.
3. El Postgres del control plane (instalación) es distinto de estas DBs de app.
