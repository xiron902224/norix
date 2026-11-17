# Primeros pasos

Norix ya debe estar instalado y accesible en el navegador (p. ej. `http://TU_IP:8000`).

## 1. Primer admin

En una instalación nueva la UI pide crear el **primer admin** (bootstrap).

El bootstrap solo funciona por **HTTPS** (proxy de confianza) o desde **loopback** (`http://127.0.0.1:…`). Por HTTP público la API rechaza la creación salvo `NORIX_ALLOW_INSECURE_BOOTSTRAP=1` (labs / Docker one-click; quítalo tras TLS).

```bash
# Túnel SSH y luego abre http://127.0.0.1:8000
ssh -L 8000:127.0.0.1:8000 user@TU_HOST
```

O TLS delante del panel — [TLS en producción](./production-tls.md).

Si ya hay cuenta: **Login** (email/password u OIDC si está configurado).

## 2. Verificar el control plane

- Sidebar: versión bajo el logo **N**
- `curl -sf https://TU_CP/status` → `"status":"operational"`

## 3. Añadir un worker

Builds Git e imágenes necesitan un nodo **online** con **Docker**.

1. **Regions & nodes** → crear join token  
2. En el worker ejecuta el comando de la UI (detalle en [Nodos](./nodes-agents.md))  
3. Confirma **online** y **Docker = yes**

## 4. Proyecto y deploy

1. **Projects** → crear proyecto  
2. Crear workload  
3. Deploy con imagen (`nginx:alpine`) o **Deploy from Git**  
4. Abre la URL/puerto o asigna un [dominio](./domains.md)

## 5. Siguiente

Dominios · bases de datos · [Ops](./ops.md) · [actualizar agents](../../UPDATE_AND_UNINSTALL.md)
