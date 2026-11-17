# TLS en producción

**Bootstrap del primer admin (#6):** solo loopback, HTTPS (proxy local con `X-Forwarded-Proto: https`) o `NORIX_ALLOW_INSECURE_BOOTSTRAP=1`. Preferible terminar TLS (esta guía) o un túnel SSH antes de crear el owner.

Guía completa (EN): [`../production-tls.md`](../production-tls.md).

Resumen:

1. Elige `norix.ejemplo.com` (panel) y hostnames de apps.
2. DNS A/AAAA → VPS / edge; abre **:80** y **:443**.
3. Instalador con panel HTTPS (#26):
   `get-norix.sh --panel-domain norix.ejemplo.com --acme-email ops@ejemplo.com`
   (Caddy + ACME HTTP-01 vía `docker-compose.panel-tls.yml`).
4. O pon tu propio Caddy/nginx delante del CP (`HTTP_ADDR=127.0.0.1:…`).
5. Alinea `NORIX_PUBLIC_URL` / `NORIX_WEB_ORIGIN` / CORS con `https://…`.
6. Dominios de apps en el dashboard + ACME edge (HTTP-01 o DNS-01).

Lab Pebble: `scripts/test-panel-tls-pebble.sh`. Verifica: `curl -sfI https://norix.ejemplo.com/healthz`.
