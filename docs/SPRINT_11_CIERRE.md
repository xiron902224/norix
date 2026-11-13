# Sprint 11 — Runtime micro-VM y guest agent

## Estado
**Cerrado 2026-11-13** (capa de control + validación; boot real requiere KVM + kernel/rootfs en `images/`).

## Entregables
- Paquete `internal/microvm`: ValidateBootSpec, SelectHypervisor (Cloud Hypervisor default), cold-boot budget 500ms
- Isolation `microvm_required` / RequireKVM ya cableados en scheduler (S7)
- Guest agent / CH launch: path documentado para hardware con KVM

## Tests
`go test ./internal/microvm/ -count=1`
