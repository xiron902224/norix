DROP TABLE IF EXISTS platform_certificates;
DROP TABLE IF EXISTS network_dns_records;
DROP TABLE IF EXISTS workload_egress_policies;
DROP TABLE IF EXISTS project_network;

ALTER TABLE nodes
    DROP COLUMN IF EXISTS wg_public_key,
    DROP COLUMN IF EXISTS wg_private_key_enc,
    DROP COLUMN IF EXISTS wg_private_key_nonce,
    DROP COLUMN IF EXISTS wg_address,
    DROP COLUMN IF EXISTS wg_listen_port,
    DROP COLUMN IF EXISTS network_status,
    DROP COLUMN IF EXISTS network_last_error,
    DROP COLUMN IF EXISTS network_generation,
    DROP COLUMN IF EXISTS network_observed_generation;
