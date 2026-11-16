# TLS en producción

Guía completa (EN): [`../production-tls.md`](../production-tls.md).

Resumen:

1. Elige `norix.ejemplo.com` (panel) y hostnames de apps.
2. DNS A/AAAA → VPS / edge.
3. Pon Caddy/nginx delante del CP (`HTTP_ADDR=127.0.0.1:…`) con Let’s Encrypt.
4. Alinea `NORIX_PUBLIC_URL` / `NORIX_WEB_ORIGIN` / CORS con `https://…`.
5. Dominios de apps en el dashboard + ACME edge o TLS en tu proxy.

Verifica: `curl -sfI https://norix.ejemplo.com/healthz`.
