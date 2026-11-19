# Stacks de Compose

Un **stack de compose** es un `docker-compose.yml` de Git que Norix ejecuta como una sola unidad. El agente clona el repositorio y ejecuta `docker compose up` con el archivo tal cual. Los volúmenes, `command`, `entrypoint`, healthchecks, condiciones de `depends_on`, trabajos con `restart: "no"` y los bind mounts relativos se comportan igual que en tu laptop.

## Desplegar un stack

1. Abre un proyecto → **Compose stack**.
2. Rellena **Stack name**, **Git URL**, **Ref** y **Compose file** (ruta dentro del repo, por ejemplo `deploy/docker-compose.yml`).
3. Opcional, en el mismo formulario:
   - **Public services**: nombres de servicio separados por comas que mantienen sus puertos publicados en el host. Todos los demás son internos.
   - **Domains**: un `servicio=hostname` por línea. Norix enruta ese hostname a ese servicio por HTTPS/WSS (ver [Dominios](#dominios)).
   - **Variables**: un `NOMBRE=valor` por línea para la interpolación `${NOMBRE}` (ver [Variables](#variables)).
4. Elige:
   - **Deploy stack**: lo encola ahora. Úsalo para repositorios privados que solo el agente puede clonar (clave SSH en el nodo).
   - **Analyze compose**: el control plane clona el repo y lista primero los servicios. Necesita un repositorio que el control plane pueda alcanzar.
5. La tarjeta del stack muestra el estado y cada servicio. Un nodo con Docker lo toma en su siguiente heartbeat.

Lo que ejecuta el agente, en `<workdir del agente>/stacks/<prefijo del stack id>/`:

```bash
git clone --depth 1 --branch <ref> <url> src     # despliegues siguientes: fetch + reset --hard
docker compose -p norix-<prefijo del stack id> --env-file stack.env \
  -f src/<archivo compose> -f norix.override.yml \
  up -d --build --remove-orphans
```

`norix.override.yml` lo genera Norix. Etiqueta cada servicio, se une a la red del proyecto, fija los aliases de servicio y quita los puertos del host de los servicios internos. `stack.env` contiene las variables para la interpolación. Se escribe con modo `0600` y se borra tras cada aplicación. El checkout se conserva entre despliegues para que los bind mounts relativos mantengan sus datos.

## Estado

| Estado del stack | Significado |
|---|---|
| `pending` | Esperando un nodo, o el siguiente heartbeat tras una acción. |
| `deploying` | `docker compose up` está en marcha, o un servicio sigue en `created` / health `starting`. |
| `running` | Todos los servicios están corriendo y no están unhealthy. Un trabajo de una sola vez que terminó con código 0 cuenta como **Completed**. |
| `degraded` | Un servicio está reiniciando, unhealthy o terminó con código distinto de cero. |
| `stopped` | El stack se detuvo o no existe ningún contenedor. |
| `failed` | La última aplicación falló (clone, `compose config`, variable faltante, bind fuera de la allowlist, build o `up`). La tarjeta muestra el error. Corrígelo y pulsa **Redeploy**. |
| `removing` | `docker compose down` está en marcha. |

El build y el `up` corren en segundo plano, así que el agente sigue enviando heartbeats durante builds largos. El timeout por defecto es una hora. Cámbialo con `NORIX_COMPOSE_UP_TIMEOUT_SECS` en el agente (60–21600).

## Red

Todos los stacks de un proyecto comparten una red Docker, `norix-p-<id del proyecto en hex>`, en el mismo nodo. Cada servicio es alcanzable desde otros stacks del proyecto con dos nombres:

- `<servicio>`: el nombre del archivo compose, por ejemplo `db`.
- `<proyecto compose>-<servicio>`, por ejemplo `norix-3f2504e0-db`. Usa este cuando quieras un nombre que no pueda chocar.

Un stack también puede unirse a una red que ya existe en el nodo. Decláralo en el archivo compose como `external: true`. Norix conserva los aliases que definas en esa red. Así un stack puede llegar a una base de datos compartida sin publicar un puerto.

## Servicios públicos e internos

Los servicios son **internos por defecto**: Norix quita sus puertos del host (`ports: !reset []`), así que solo son alcanzables en la red del proyecto. Pon un servicio en **Public services** para conservar sus puertos publicados tal como están en el archivo compose.

## Dominios

Un dominio envía el tráfico HTTPS (y WebSocket `wss://`) de un hostname a un servicio:

```text
web=app.example.com
api=api.example.com
```

Requisitos:

- El servicio debe declarar un puerto de contenedor en el archivo compose (`ports` o `expose`). Sin puerto, Norix omite el dominio.
- El nodo que ejecuta el stack debe tener también el edge Caddy local (el contenedor con la etiqueta `caddy`). Norix une Caddy a la red del proyecto y enruta a `http://norix-<id>-<servicio>:<puerto>`.
- El registro DNS del hostname debe apuntar al edge del nodo. Caddy gestiona el certificado TLS.

Las rutas se reemplazan completas en cada plan del edge, así que un stack parado o eliminado sale de Caddy en el siguiente plan.

## Variables

Define variables en el formulario o en **Settings** de la tarjeta. Los valores son de solo escritura:

- Los valores se envían una vez y se guardan cifrados (AES-GCM). La API devuelve solo nombres, y la tarjeta los lista sin valores.
- Para cambiar un valor, vuelve a enviarlo. Para borrar una variable, envía `NOMBRE=` (valor vacío).
- La interpolación funciona como siempre: `${DB_PASSWORD}`, `${DB_PASSWORD:-default}`. Una variable obligatoria que falta (`${DB_PASSWORD:?required}`) hace fallar la aplicación y nombra la variable.
- `SERVICE_PASSWORD_<NOMBRE>` (32 caracteres alfanuméricos) y `SERVICE_BASE64_<NOMBRE>` (32 bytes aleatorios en base64) se **generan una sola vez** cuando el archivo los referencia, y se reutilizan en cada despliegue posterior. Sus valores no se muestran en la UI. Se guardan cifrados en el control plane y en `generated.json` (modo `0600`) en el nodo.

Norix pasa sus variables con `--env-file`, así que un `.env` versionado en el repo **no** se lee. Pon los valores por defecto no secretos en el archivo compose (`${VAR:-default}`).

## Volúmenes y borrado

La tarjeta lista cada volumen que usa el stack como `named` o `bind`.

| Botón | Ejecuta el agente |
|---|---|
| **Start** / **Redeploy** | `git fetch` + `docker compose up -d --build --remove-orphans` |
| **Stop** | `docker compose stop` |
| **Delete** | `docker compose down --remove-orphans`. **Los volúmenes nombrados y el checkout se conservan.** |
| **Delete** con **Purge volumes on delete** marcado | `docker compose down --remove-orphans --volumes`, y después borra el checkout. **No se puede deshacer.** |

Otras reglas:

- Un bind mount debe quedar dentro del checkout o dentro de una ruta de `NORIX_COMPOSE_BIND_ALLOWLIST` del agente (rutas absolutas separadas por comas). Si no, la aplicación falla antes de `up`.
- Si `git fetch` falla, el agente conserva el checkout y reporta el error. Nunca borra datos de bind para recuperarse.

Cada acción incrementa la generación del stack (`gen observed/desired` en la tarjeta).

## Logs

**Logs** en la tarjeta muestra las últimas líneas por servicio, más `deploy output` (clone, build y `up`). Las líneas siguen la política de [retención de logs](./ops.md#log-retention): cada servicio cuenta como un workload. Los valores secretos se redactan de la salida.

## Ubicación

Todos los stacks de un proyecto corren en el **mismo nodo**, así que pueden compartir la red del proyecto. Un stack se queda en su nodo. Norix no lo mueve, porque sus volúmenes viven ahí.

## Repositorios privados

El agente clona como el usuario que ejecuta `norix-agent`, con `GIT_TERMINAL_PROMPT=0`. Para repos privados de GitHub/GitLab, añade una deploy key de solo lectura en `~/.ssh` de ese usuario y usa una URL SSH (`git@github.com:org/repo.git`). No pongas tokens en la URL: acaban en `.git/config` del nodo. Norix redacta `user:token@` de la salida de git en los logs.

## Legado: dividir en workloads

Tras **Analyze compose**, **Split into workloads (legacy)** sigue convirtiendo cada servicio en un workload de Norix separado (comportamiento antiguo). Ese modo ignora volúmenes, comandos, healthchecks, `restart`, y las funciones de red y variables de arriba. Úsalo solo para stacks simples de solo imágenes que necesiten escalado o rolling updates por servicio.
