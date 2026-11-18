# Stacks Compose

Un **stack compose** es un `docker-compose.yml` de Git que Norix ejecuta como una unidad. El agente clona el repositorio y lanza `docker compose up` con el fichero tal cual. Volúmenes, `command`, `entrypoint`, healthchecks, condiciones de `depends_on`, jobs con `restart: "no"`, ficheros de secrets y bind mounts relativos funcionan igual que en tu portátil.

## Desplegar un stack

1. Abre un proyecto → **Compose stack**.
2. Rellena **Stack name**, **Git URL**, **Ref** y **Compose file** (ruta dentro del repo, por ejemplo `deploy/docker-compose.yml`).
3. Elige:
   - **Deploy stack**: lo encola ya. Úsalo con repos privados que solo el agente puede clonar (clave SSH en el nodo).
   - **Analyze compose**: el control plane clona el repo y lista antes los servicios. Necesita un repo al que llegue el control plane.
4. La ficha del stack muestra el estado y cada servicio. Un nodo con Docker reclama el stack en su siguiente heartbeat.

Lo que ejecuta el agente, en `<workdir del agente>/stacks/<id del stack>/`:

```bash
git clone --depth 1 --branch <ref> <url> src     # siguientes deploys: fetch + reset --hard
docker compose -p norix-<id> -f src/<compose file> -f norix.override.json \
  up -d --build --remove-orphans
```

`norix.override.json` solo añade etiquetas (`norix.stack_id`, `norix.project_id`). El checkout se conserva entre deploys para que los bind mounts relativos no pierdan datos.

## Estados

| Estado | Significado |
|---|---|
| `pending` | Espera un nodo, o el siguiente heartbeat tras una acción. |
| `deploying` | `docker compose up` en curso, o un servicio sigue `created` / health `starting`. |
| `running` | Todos los servicios corren y ninguno está unhealthy. Un job de una sola ejecución que salió con código 0 cuenta como **Completed**. |
| `degraded` | Un servicio se reinicia, está unhealthy o salió con código distinto de 0. |
| `stopped` | El stack se paró, o no existe ningún contenedor. |
| `failed` | Falló el último apply (clone, `compose config`, build o `up`). La ficha muestra el error. Arregla el repo y pulsa **Redeploy**. |
| `removing` | `docker compose down` en curso. |

El build y el `up` corren en segundo plano, así que el agente sigue enviando heartbeats durante builds largos. El timeout por defecto es una hora. Cámbialo con `NORIX_COMPOSE_UP_TIMEOUT_SECS` en el agente (60–21600).

## Acciones

| Botón | El agente ejecuta |
|---|---|
| **Start** / **Redeploy** | `git fetch` + `docker compose up -d --build --remove-orphans` |
| **Stop** | `docker compose stop` |
| **Delete** | `docker compose down --remove-orphans` y borra el checkout. **Los volúmenes con nombre se conservan.** |

Cada acción sube la generación del stack (`gen observed/desired` en la ficha).

## Logs

**Logs** en la ficha muestra las últimas líneas por servicio y `deploy output` (clone, build y `up`). Siguen la [retención de logs](./ops.md#retención-de-logs): cada servicio cuenta como un workload.

## Ubicación

Todos los stacks de un proyecto corren en el **mismo nodo**, para que más adelante puedan compartir redes Docker entre stacks. Un stack se queda en su nodo. Norix no lo mueve porque sus volúmenes viven ahí.

## Repos privados

El agente clona con el usuario que ejecuta `norix-agent` y con `GIT_TERMINAL_PROMPT=0`. Para repos privados de GitHub/GitLab, añade una deploy key de solo lectura en el `~/.ssh` de ese usuario y usa una URL SSH (`git@github.com:org/repo.git`). No metas tokens en la URL: acaban en `.git/config` del nodo. Norix oculta `user:token@` en la salida de git que registra.

## Pendiente

| Necesidad | Issue |
|---|---|
| Red compartida entre stacks y redes `external: true` con alias | [#144](https://github.com/xiron902224/norix/issues/144) |
| Variables del stack para interpolar `${VAR}` y secrets generados | [#145](https://github.com/xiron902224/norix/issues/145) |
| Servicios internos por defecto; dominios con HTTPS/WSS por servicio | [#146](https://github.com/xiron902224/norix/issues/146) |
| Borrar volúmenes al eliminar | [#147](https://github.com/xiron902224/norix/issues/147) |

Hasta que llegue #145, pon defaults en el compose (`${VAR:-default}`) o versiona un `.env` sin secretos junto al fichero.

## Legacy: dividir en workloads

Tras **Analyze compose**, **Split into workloads (legacy)** sigue convirtiendo cada servicio en un workload Norix separado (comportamiento antiguo). Ese modo ignora volúmenes, comandos, healthchecks y `restart`. Úsalo solo para stacks simples de imágenes que necesiten escalado o rolling por servicio.
