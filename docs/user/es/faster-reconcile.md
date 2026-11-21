# Reconcile más rápido (#44)

Antes, el estado deseado solo viajaba en el heartbeat del agent (default **10 s**). Tras un deploy o stop, la convergencia esperaba hasta un intervalo completo.

Ahora el agent abre `WatchDesired` (long-poll) en paralelo: el control plane responde al cambiar el fingerprint deseado del nodo, o al timeout (~25 s). Los mutadores despiertan a los waiters. Al reconectar hay backoff y fingerprint para no repetir applies.

Detalle y medición: [guía EN](../faster-reconcile.md).
