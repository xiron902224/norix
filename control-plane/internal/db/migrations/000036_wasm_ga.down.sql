ALTER TABLE wasm_functions DROP CONSTRAINT IF EXISTS wasm_functions_status_check;
ALTER TABLE wasm_functions
    ADD CONSTRAINT wasm_functions_status_check
    CHECK (status IN ('pending', 'ready', 'failed'));

ALTER TABLE wasm_functions
    DROP COLUMN IF EXISTS listen_port,
    DROP COLUMN IF EXISTS hostname;
