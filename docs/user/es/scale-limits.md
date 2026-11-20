# Límites de escala

Topes duros por organización (#45). Los defaults son la escala soportada en beta.6; súbelos con variables de entorno del control plane.

| Recurso | Default | Variable |
|---|---:|---|
| Nodos por organización | 32 | `NORIX_MAX_NODES_PER_ORG` |
| Proyectos por organización | 100 | `NORIX_MAX_PROJECTS_PER_ORG` |
| Workloads por proyecto | 100 | `NORIX_MAX_WORKLOADS_PER_PROJECT` |
| Réplicas por workload | 32 | `NORIX_MAX_REPLICAS` |
| Máximo del autoscaler | 8 | `NORIX_MAX_AUTOSCALER_REPLICAS` (también acotado por `NORIX_MAX_REPLICAS`) |

Pasar el tope responde invalid-argument y nombra la variable. Reinicia el control plane tras cambiarlas. Huella de binarios: [footprint.md](./footprint.md).
