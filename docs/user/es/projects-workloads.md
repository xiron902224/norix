# Proyectos y workloads

**Proyecto** = agrupación de apps. **Workload** = unidad desplegable (imagen, Git, Compose, etc.).

1. **Projects** → crear proyecto  
2. Abrir → crear workload (nombre, imagen, env, límites, puertos)  
3. Deploy / logs / dominios desde la ficha del workload  

Antes de retirar un nodo: **drain**. Los secrets no van en Git.

Las **env del workload se cifran en reposo** (AES-GCM, `NORIX_BUILD_SECRETS_KEY`). API/UI muestran `********`; el agent recibe el valor real al aplicar contenedores. Rotación de key: [S7 operations](../../runbooks/S7_OPERATIONS.md).
