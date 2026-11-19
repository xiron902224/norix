DROP TABLE IF EXISTS compose_stack_variables;
ALTER TABLE compose_stacks
    DROP COLUMN IF EXISTS edge_pending,
    DROP COLUMN IF EXISTS purge_volumes,
    DROP COLUMN IF EXISTS volumes,
    DROP COLUMN IF EXISTS domains,
    DROP COLUMN IF EXISTS public_services;
