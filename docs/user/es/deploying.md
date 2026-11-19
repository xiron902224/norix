# Desplegar

## Requisitos

≥1 nodo online con Docker/containerd. Si no, el error es `no online node with Docker or containerd`.

## Imagen

Workload → imagen (p. ej. `nginx:alpine`) → **Deploy**.

## Git

URL + ref + Dockerfile → **Deploy from Git** (build en un nodo con Docker, luego schedule).

### Railpack (sin Dockerfile)

Si el repo **no** tiene Dockerfile / Containerfile / compose, Norix usa **[Railpack](https://railpack.com/)** al detectar `package.json`, `go.mod`, `requirements.txt` / `pyproject.toml` / `Pipfile`, o `index.html`. El nodo builder necesita `railpack` en `PATH` y BuildKit (`BUILDKIT_HOST` o contenedor `norix-buildkit`):

```bash
curl -sSL https://railpack.com/install.sh | sh
```

Ejemplos: [`examples/`](../../../examples/).

Por defecto solo se permiten remotes `https://`, `ssh://` y `git@host:path`. Se rechazan `http://`/`file://`/`git://` y hosts que resuelvan a loopback, RFC1918, link-local, CGNAT o ULA (SSRF). Para Git privado en lab:

```bash
export NORIX_GIT_SSRF_ALLOWLIST=git.interno.example,10.10.0.0/16
```

(En control-plane y agent. Detalle en la [guía EN](../deploying.md#git-url-ssrf-controls).)

### Concurrencia y límites de builds (#24)

Los builds corren en paralelo en cada nodo, hasta `NORIX_BUILD_CONCURRENCY` (por defecto `2`, entre 1 y 8), tanto en el agente como en el runtime embebido. Un build por encima del límite espera y muestra `queued` en la pestaña **Build**, con una línea de log como `queued: 2 of 2 build slots busy`.

| Variable | Por defecto | Efecto |
|---|---|---|
| `NORIX_BUILD_CONCURRENCY` | `2` | Builds que corren a la vez en un nodo |
| `NORIX_BUILD_TIMEOUT_SECS` | `3600` | Límite de tiempo real por build (60–21600). Al vencer, el build falla y se matan sus procesos git/docker |
| `NORIX_BUILDKIT_CPUS` / `NORIX_BUILDKIT_MEMORY` | sin definir | Límites de CPU y memoria (por ejemplo `2` y `4g`) para el contenedor BuildKit compartido de Railpack. Se aplican cuando Norix arranca ese contenedor |

`docker build` corre en el daemon de Docker y estas variables no lo limitan. Para esa vía usa los límites propios de Docker.

## Stacks Compose

Un `docker-compose.yml` de Git corre como una unidad con `docker compose up` en un agente, con volúmenes, comandos, healthchecks y jobs de una sola ejecución. Ver [Stacks Compose](./compose-stacks.md).

## Parar y reiniciar

En la página de la aplicación:

- **Stop** conserva el workload y pide al agent que borre el contenedor. **Start** lo vuelve a desplegar.
- **Restart** y **Redeploy** encolan otro deploy (generación + 1). El agent recrea el contenedor.
- **Pause** no está. Parar sin borrar la fila es **Stop**.

## Después

Puerto publicado o [dominio](./domains.md) con TLS.
