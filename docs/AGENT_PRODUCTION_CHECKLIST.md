# Norix Agent — Checklist de producción

Este documento recoge la hoja de ruta mínima para dejar el agente de Norix en una base operativa y cercana a producción.

## Objetivo

Cerrar el agente como un reconciliador fiable del nodo, con validación de arranque, heartbeat, runtime seguro y pruebas reales end-to-end antes de continuar con el control plane y la CLI.

---

## Prioridad real

1. Reconciliación segura y idempotente
2. Enroll + heartbeat + recovery
3. Runtime failure / degraded mode
4. E2E real con control plane
5. Packaging / install / observabilidad

---

## Sprint A — Base de producción del agente

### Alcance

Hacer que el agente arranque solo si está bien configurado y falle con mensajes claros.

### Archivos clave
- `agent/src/config.rs`
- `agent/src/main.rs`

### Tareas
- [ ] Validar `NORIX_CONTROL_PLANE_URL` no vacía
- [ ] Validar token de registro o estado guardado
- [ ] Crear el directorio de trabajo si no existe
- [ ] Añadir modo `--check` para validación de arranque
- [ ] Definir logs claros para startup, enroll y runtime
- [ ] Evitar que el agente se inicie en un estado ambiguo

### Pruebas mínimas
- [ ] config invalid without token
- [ ] config valid with saved state
- [ ] startup check fails fast
- [ ] startup check succeeds when runtime is available

### DoD
- [ ] El agente no arranca a ciegas
- [ ] Si falta configuración, falla con mensaje accionable

---

## Sprint B — Enroll + heartbeat + desired state

### Alcance

Que el agente se registre correctamente y reciba deseos del control plane.

### Archivos clave
- `agent/src/client.rs`
- `agent/src/main.rs`

### Tareas
- [ ] Enroll con token de unión
- [ ] Guardar `node-state.json` tras enroll correcto
- [ ] Heartbeat con CPU, memoria y capacidades
- [ ] Parseo correcto de `desired_workloads`, `desired_builds`, `desired_edge`, `desired_network`
- [ ] Manejar errores HTTP 401/403/500
- [ ] Backoff exponencial en fallos de heartbeat
- [ ] Reintentar sin romper el ciclo principal del agente

### Pruebas mínimas
- [ ] enroll success with token
- [ ] persist state after enroll
- [ ] heartbeat response with desired workloads
- [ ] invalid JSON is handled
- [ ] retry/backoff path

### DoD
- [ ] El agente se vuelve a registrar de forma segura
- [ ] Si el control plane cae, reintenta sin romper el loop

---

## Sprint C — Reconciliación real del runtime

### Alcance

Que un workload real se cree, mantenga y reporte correctamente.

### Archivos clave
- `agent/src/runtime.rs`

### Tareas
- [ ] Comparar imagen con criterio seguro
- [ ] Evitar falsos positivos por string matching
- [ ] Crear contenedor si no existe
- [ ] Recrear si cambió la generación
- [ ] No tocar contenedores ya correctos
- [ ] Capturar logs del contenedor
- [ ] Reportar estado observado con status correcto
- [ ] Stop/cleanup idempotente

### Pruebas mínimas
- [ ] image match exact
- [ ] false positive rejected
- [ ] create workload
- [ ] no-op when generation matches
- [ ] stop removes workload cleanly
- [ ] logs are captured

### DoD
- [ ] El agente no recrea contenedores por un nombre parecido
- [ ] El estado observado refleja la realidad del nodo

---

## Sprint D — Failure / recovery / degraded mode

### Alcance

Que el agente sea operativo bajo fallos reales del entorno y del control plane.

### Archivos clave
- `agent/src/main.rs`
- `agent/src/runtime.rs`

### Tareas
- [ ] Si Docker/no runtime existe, seguir en modo degraded
- [ ] Si heartbeat falla, usar backoff
- [ ] Si report observed falla, reintentar
- [ ] Si un workload falla al arrancar, reportar error real
- [ ] Si el control plane vuelve, recuperar sin reinicio manual
- [ ] Mantener el bucle vivo y observaciones coherentes

### Pruebas mínimas
- [ ] runtime unavailable mode
- [ ] heartbeat transient failure
- [ ] build/report retry path
- [ ] workload failed status is surfaced
- [ ] recovery after API outage

### DoD
- [ ] El nodo se recupera solo
- [ ] El control plane recibe estados fiables

---

## Sprint E — E2E con control plane real

### Alcance

Validar el agente con la infraestructura real del proyecto.

### Tareas
- [ ] Levantar control plane local
- [ ] Crear nodo y token de registro
- [ ] Ejecutar agente real
- [ ] Crear workload desde API
- [ ] Verificar que el agent lo levanta
- [ ] Verificar que el control plane recibe `observed`
- [ ] Probar actualización de workload y re-reconciliación
- [ ] Probar fallo y recuperación

### Pruebas mínimas
- [ ] enroll end-to-end
- [ ] desired workload reaches host
- [ ] observed status reaches control plane
- [ ] workload update triggers reconcile
- [ ] failed workload is reported

### DoD
- [ ] Flujo completo de extremo a extremo funcionando

---

## Pruebas obligatorias antes de beta

Estas pruebas no pueden faltar antes de declarar beta del agente:

- [ ] config validation
- [ ] persisted state load
- [ ] enroll success and failure
- [ ] heartbeat success and retry
- [ ] desired workload apply
- [ ] runtime unavailable scenario
- [ ] image identity correctness
- [ ] Docker/nerdctl reconcile
- [ ] observed status generation
- [ ] error propagation to control plane
- [ ] recovery after transient outage

---

## Recomendación técnica

- Agente: Rust
- Control plane: Go
- CLI: Go

Esto mantiene la separación adecuada:
- Rust para nodo y runtime
- Go para API, persistencia y orquestación central
- Go para tooling CLI y scripts operativos

---

## Siguiente paso práctico

Comenzar por:

1. cerrar pruebas de reconciliación real en `agent/src/runtime.rs`
2. cerrar recovery/backoff en `agent/src/main.rs`
3. montar smoke test end-to-end con el control plane

Solo cuando esas 3 cosas estén cubiertas conviene pasar al siguiente bloque: control plane y CLI.
