-- S9 observability: time-series metrics, log segments, alert rules/events.

ALTER TABLE alert_events RENAME TO ops_alert_events;
ALTER INDEX IF EXISTS alert_events_org_created_idx RENAME TO ops_alert_events_org_created_idx;
ALTER SEQUENCE alert_events_id_seq RENAME TO ops_alert_events_id_seq;

DROP POLICY IF EXISTS alert_events_tenant_policy ON ops_alert_events;
CREATE POLICY ops_alert_events_tenant_policy ON ops_alert_events USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

CREATE TABLE IF NOT EXISTS metric_samples (
    id BIGSERIAL PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    value DOUBLE PRECISION NOT NULL,
    ts TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS log_segments (
    id BIGSERIAL PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    workload_id UUID,
    body TEXT NOT NULL,
    level TEXT NOT NULL DEFAULT 'info'
        CHECK (level IN ('debug', 'info', 'warn', 'error')),
    ts TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    threshold DOUBLE PRECISION NOT NULL DEFAULT 0,
    channel TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, name)
);

CREATE TABLE IF NOT EXISTS alert_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    rule_id UUID NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    message TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'acknowledged', 'resolved')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS metric_samples_org_ts_idx ON metric_samples (organization_id, ts DESC);
CREATE INDEX IF NOT EXISTS metric_samples_org_name_idx ON metric_samples (organization_id, name, ts DESC);
CREATE INDEX IF NOT EXISTS log_segments_org_ts_idx ON log_segments (organization_id, ts DESC);
CREATE INDEX IF NOT EXISTS alert_events_org_created_idx ON alert_events (organization_id, created_at DESC);

ALTER TABLE metric_samples ENABLE ROW LEVEL SECURITY;
ALTER TABLE metric_samples FORCE ROW LEVEL SECURITY;
ALTER TABLE log_segments ENABLE ROW LEVEL SECURITY;
ALTER TABLE log_segments FORCE ROW LEVEL SECURITY;
ALTER TABLE alert_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_rules FORCE ROW LEVEL SECURITY;
ALTER TABLE alert_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_events FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS metric_samples_tenant_policy ON metric_samples;
CREATE POLICY metric_samples_tenant_policy ON metric_samples USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

DROP POLICY IF EXISTS log_segments_tenant_policy ON log_segments;
CREATE POLICY log_segments_tenant_policy ON log_segments USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

DROP POLICY IF EXISTS alert_rules_tenant_policy ON alert_rules;
CREATE POLICY alert_rules_tenant_policy ON alert_rules USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

DROP POLICY IF EXISTS alert_events_tenant_policy ON alert_events;
CREATE POLICY alert_events_tenant_policy ON alert_events USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON metric_samples, log_segments, alert_rules, alert_events TO norix_app;
GRANT USAGE, SELECT ON SEQUENCE metric_samples_id_seq TO norix_app;
GRANT USAGE, SELECT ON SEQUENCE log_segments_id_seq TO norix_app;
