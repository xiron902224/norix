# Proyectos y workloads

**Proyecto** = agrupación de apps. **Workload** = unidad desplegable (imagen, Git, Compose, etc.).

1. **Projects** → crear proyecto  
2. Abrir → crear workload (nombre, imagen, env, límites, puertos)  
3. Deploy / logs / dominios desde la ficha del workload  

### Variables de entorno (dashboard)

1. **Crear** — Projects → **New application** (imagen o git) → **Environment variables** → **Add** → clave/valor (p. ej. `FOO` / `secret`) → Create.
2. **Ver** — Application → **Summary**: keys con valor `********` (nunca plaintext).
3. **Editar** — pestaña **Edit** → cambiar el valor o dejar `********` para conservar el secreto → **Save changes**. Quitar la fila elimina esa key.

Se guardan cifradas en reposo; la API trata `********` como “sin cambio”. UX completa (import `.env`, reveal auditado): [#23](https://github.com/xiron902224/norix/issues/23).

Antes de retirar un nodo: **drain**. Los secrets no van en Git.

Las **env del workload se cifran en reposo** (AES-GCM, `NORIX_BUILD_SECRETS_KEY`). API/UI muestran `********`; el agent recibe el valor real al aplicar contenedores. Rotación de key: [S7 operations](../../runbooks/S7_OPERATIONS.md).
