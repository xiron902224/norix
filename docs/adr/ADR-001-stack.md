# ADR-001 — Stack base de Norix

Estado: aceptado

## Contexto

Hace falta fijar el stack de control plane, agente, API y dashboard para no reabrir el debate en cada sprint.

## Decisión

- Control plane: Go + ConnectRPC + PostgreSQL
- Agente de nodo: Rust (Tokio), a partir de S3
- CLI de usuario (`norix`): **Rust** (clap) — ver [ADR-005](./ADR-005-rust-cli.md) / #28
- Dashboard: React 19 + Vite SPA embebida con `go:embed`
- Contratos: Buf / protobuf compartidos

## Alternativas

- Next.js SSR para el panel (descartado: proceso Node extra en prod)
- gRPC puro sin Connect (peor DX web)
- Agente en Go (posible si la fricción Rust/CI lo exige; se revisa en S4)
- CLI en Go/Cobra (descartado en favor de Rust; ADR-005)

## Consecuencias

Un binario de control plane sirve API + UI. El agente se desarrolla aparte con los mismos protos.
