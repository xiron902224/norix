# ADR-002 — PostgreSQL-only hasta multi-nodo

Estado: aceptado

## Contexto

El plan original pedía SQLite + Postgres con sqlc. El código actual usa `pgx` y solo PostgreSQL.

## Decisión

v1 usa **solo PostgreSQL**. sqlc es opcional y se evaluará tras estabilizar el modelo desired/observed (post-S4). SQLite no es objetivo de v1.0.

## Alternativas

- Dual SQLite/Postgres desde el día 1 (más superficie, poco beneficio local con Compose)
- Adoptar sqlc ahora (reescribe capa DB sin ganar runtime)

## Consecuencias

Menos abstracción, migraciones SQL directas, RLS real desde el MVP. Documentos que mencionen SQLite/sqlc como obligatorio quedan obsoletos.
