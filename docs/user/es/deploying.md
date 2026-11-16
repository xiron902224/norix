# Desplegar

## Requisitos

≥1 nodo online con Docker/containerd. Si no, el deploy queda pending / unschedulable.

## Imagen

Workload → imagen (p. ej. `nginx:alpine`) → **Deploy**.

## Git

URL + ref + Dockerfile → **Deploy from Git** (build en un nodo con Docker, luego schedule).

## Después

Puerto publicado o [dominio](./domains.md) con TLS.
