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

## Después

Puerto publicado o [dominio](./domains.md) con TLS.
