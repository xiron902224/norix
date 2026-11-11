# ADR-001 — Stack base de Norix

Estado: aceptado

## Contexto

Hace falta fijar el stack de control plane, agente, API y dashboard para no reabrir el debate en cada sprint.

## Decisión

- Control plane: Go + ConnectRPC + PostgreSQL
- Agente de nodo: Rust (Tokio), a partir de S3
- Dashboard: React 19 + Vite SPA embebida con `go:embed`
- Contratos: Buf / protobuf compartidos

## Alternativas

- Next.js SSR para el panel (descartado: proceso Node extra en prod)
- gRPC puro sin Connect (peor DX web)
- Agente en Go (posible si la fricción Rust/CI lo exige; se revisa en S4)

## Consecuencias

Un binario de control plane sirve API + UI. El agente se desarrolla aparte con los mismos protos.
