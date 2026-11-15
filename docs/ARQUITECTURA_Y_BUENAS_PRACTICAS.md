# Arquitectura y buenas prácticas de Norix

Este documento define dónde colocar código y cómo conectar frontend, backend y agente Rust.

## Objetivos

- Responsabilidades fáciles de localizar.
- Dependencias dirigidas hacia el dominio, no hacia frameworks.
- Código Go y Rust con errores explícitos.
- Frontend dividido por componentes reutilizables y features.
- Comunicación mantenida mediante contratos Buf.

## Frontend — Atomic Design

```text
web/src/
├── features/
│   └── auth/
│       ├── components/
│       ├── hooks/
│       └── services/
├── shared/
│   ├── ui/
│       ├── layouts/
│       └── lib/
├── api/
│   └── auth-client.ts
└── gen/
```

### Átomos

Componentes sin lógica de negocio:

- Button.
- FormField.
- Badge.
- Spinner.
- Alert.

### Moléculas

Combinan átomos sin conocer APIs:

- PasswordField.
- LoginActions.
- UserMenu.

### Organismos

Implementan una sección completa:

- LoginForm.
- BootstrapForm.
- ProjectTable.
- DeploymentStatus.

### Templates

Distribuyen organismos sin conocer rutas concretas:

- AuthPageLayout.
- DashboardLayout.

### Páginas

Conectan templates con routing y estado del dominio:

- SetupPage.
- LoginPage.
- ProjectsPage.

## Servicios frontend

Los componentes React no deben ejecutar SQL ni crear autenticación criptográfica. Solo consumen servicios y hooks.

```text
Componente
  -> Hook
  -> Service/Connect client
  -> Control plane Go
  -> PostgreSQL
```

### `sessionStore`

Responsabilidad:

- Guardar token de sesión.
- Leer usuario actual.
- Borrar sesión al logout.

Aun así, el token nunca sustituye la validación de backend.

### Hooks

Hooks de ejemplo:

- `useBootstrapStatus`.
- `useLogin`.
- `useCurrentSession`.
- `useProjects`.

## Backend Go — Clean Architecture ligera

```text
control-plane/cmd/server
  composición, lifecycle y señales

control-plane/internal/api
  casos de uso RPC y traducción a Connect

control-plane/internal/auth
  Argon2, tokens, claims, sesiones

control-plane/internal/db
  PostgreSQL, migraciones y SQL

control-plane/internal/gen
  código generado; no editar manualmente
```

### Reglas Go

1. `cmd/server` no contiene SQL ni lógica de negocio.
2. `api` traduce errores y valida entradas RPC.
3. `auth` implementa primitives criptográficas y políticas de sesión.
4. `db` ejecuta SQL sin conocer ConnectRPC.
5. Interfaces pequeñas permiten tests sin PostgreSQL real.
6. `context.Context` cruza API, servicio y DB.
7. Cada recurso cerrable tiene `Close`.
8. No guardar secretos ni tokens completos en logs.

### Errors

Usar códigos Connect para transport:

- `InvalidArgument`: entrada incorrecta.
- `Unauthenticated`: credenciales inválidas.
- `PermissionDenied`: autorización insuficiente.
- `AlreadyExists`: recurso único creado.
- `FailedPrecondition`: estado del sistema incorrecto.
- `Internal`: error oculto al usuario, registrado internamente.

## PostgreSQL y RLS

```text
norix          -> migraciones y bootstrap privilegiado
norix_app      -> operaciones normales sin superusuario
RLS tenant     -> aislamiento por organization_id
```

Todo request autenticado debe establecer su tenant en la transacción, por ejemplo:

```sql
SELECT set_config('app.current_tenant_id', $1, true);
```

El tenant no se acepta sin verificar la membresía del usuario.

## Agente Rust — arquitectura por capas

La estructura futura:

```text
agent/src/
├── main.rs
├── config/
├── transport/
├── reconciler/
├── runtime/
│   ├── containerd/
│   ├── microvm/
│   └── wasm/
├── network/
├── capabilities/
└── observability/
```

### Reglas Rust

- Ownership explícito; evitar clones innecesarios.
- Tokio tasks aislados por responsabilidad.
- Channels para comunicación entre componentes.
- Runtime-specific logic detrás de traits pequeños.
- Errores tipados con `thiserror`.
- Tracing estructurado, sin secretos.
- Reconexión con backoff.
- Operaciones reconcile idempotentes.
- Prohibir procesos no supervised con hijos huérfanos.

### Reconciliación

```text
Estado deseado
  -> diferenciar
  -> ejecutar acción
  -> observar resultado
  -> actualizar estado
  -> repetir en caso de error temporal
```

## Contratos Buf

Son el contrato de frontera entre Go y Rust/TypeScript.

- No editar código generado.
- `buf lint` obligatorio.
- Revisar cambios incompatibles con `buf breaking`.
- Cambios de API tienen ADR si afectan arquitectura.
- Tipos protobuf evolucionan con campos nuevos, nunca reutilizando IDs.

## Seguridad por capas

1. Frontend valida UX, nunca seguridad.
2. API valida formato y tamaño.
3. Servicio aplica autorización.
4. PostgreSQL fuerza RLS y constraints.
5. Sistemas Linux aplican capacidades/cgroups/seccomp.
6. Logs nunca contienen passwords, tokens o claves privadas.

## Prueba por capa

### Go

- Unit tests: lógica y servicios.
- Integration tests: PostgreSQL real.
- Handler tests: Connect HTTP.
- Race tests para concurrencia.

### React

- Unit tests de lógica/hooks.
- Component tests con Testing Library.
- E2E Playwright para bootstrap y login.

### Rust

- Unit tests por crate.
- Integration tests por runtime.
- Tests contra procesos/containerd real.
- Fuzzing de parsers/vsock.

### Sistemas

- Healthchecks.
- Pruebas de reinicio.
- Prueba de pérdida de nodo.
- Prueba de caída PostgreSQL.
- Restore backup.
- Chaos limitado en VM de laboratorio.

## Flujo de nuevas funcionalidades

1. Crear ADR si cambia arquitectura.
2. Definir contrato protobuf.
3. Ejecutar `buf lint`.
4. Implementar dominio Go.
5. Añadir store PostgreSQL.
6. Añadir handler Connect.
7. Generar cliente TypeScript.
8. Crear hook y servicio frontend.
9. Crear átomos/moléculas/organsimos.
10. Añadir tests Go y React/Rust.
11. Ejecutar CI.
12. Documentar operación.
