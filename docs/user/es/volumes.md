# Volúmenes persistentes

Los workloads pueden declarar **volúmenes Docker con nombre**. Los datos viven en el **nodo worker fijado** (no se replican en el clúster).

## Configurar (UI)

Aplicación → **Edit** → **Volumes (JSON)** → Guardar → Deploy. Ejemplo:

```json
[{"name": "data", "mount_path": "/var/lib/app"}]
```

Opcional `read_only: true`. Nombres: `[a-zA-Z0-9._-]`, máx. 63. Las rutas de montaje deben ser absolutas y limpias (no `/`).

En **Summary**, usa **Backup** / **Delete** por volumen (hace falta nodo asignado). Delete pide confirmación y borra el volumen Docker en el nodo cuando el agent aplica la operación.

## Comportamiento

| Tema | Límite |
|---|---|
| Placement | El scheduler **fija** todas las réplicas con volúmenes al primer nodo elegido |
| Drain | Avisa con los nombres stateful; los datos **no** se mueven solos |
| Reprogramación | Las réplicas stateful **no** se reasignan si el nodo desaparece |
| Backup | El agent escribe un tar en `/var/lib/norix/volume-backups` en ese nodo |
| Borrar workload | **No** borra volúmenes Docker — usa **Delete** explícito del volumen |
| Rolling updates | Prefiere **recreate** con montajes R/W (el surge puede compartir el mismo volumen) |

Patrón de nombre Docker: `norix-wv-<12hex>-<logical>`.

## Ops

1. Prefiere **replica_count = 1** para estado escribible.
2. Haz backup antes de drain / reemplazar el nodo.
3. Ver también [Updates rolling](./rolling-updates.md) y [Nodos y agents](./nodes-agents.md).
