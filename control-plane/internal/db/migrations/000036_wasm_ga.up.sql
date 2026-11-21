-- Wasm GA (#47): listen port + hostname for edge routing; broader status values.

ALTER TABLE wasm_functions
    ADD COLUMN IF NOT EXISTS listen_port INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS hostname TEXT NOT NULL DEFAULT '';

ALTER TABLE wasm_functions DROP CONSTRAINT IF EXISTS wasm_functions_status_check;
ALTER TABLE wasm_functions
    ADD CONSTRAINT wasm_functions_status_check
    CHECK (status IN ('pending', 'ready', 'running', 'degraded', 'failed'));
