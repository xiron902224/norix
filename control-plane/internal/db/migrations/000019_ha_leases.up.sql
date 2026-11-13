-- S17: control-plane leader election leases (Postgres-backed).
CREATE TABLE IF NOT EXISTS control_plane_leases (
    name TEXT PRIMARY KEY,
    holder_id TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS control_plane_leases_expires_idx
    ON control_plane_leases (expires_at);
