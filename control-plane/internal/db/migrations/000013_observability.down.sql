DROP TABLE IF EXISTS alert_events;
DROP TABLE IF EXISTS alert_rules;
DROP TABLE IF EXISTS log_segments;
DROP TABLE IF EXISTS metric_samples;

DROP POLICY IF EXISTS ops_alert_events_tenant_policy ON ops_alert_events;
ALTER TABLE IF EXISTS ops_alert_events RENAME TO alert_events;
ALTER INDEX IF EXISTS ops_alert_events_org_created_idx RENAME TO alert_events_org_created_idx;
ALTER SEQUENCE ops_alert_events_id_seq RENAME TO alert_events_id_seq;
DROP POLICY IF EXISTS alert_events_tenant_policy ON alert_events;
CREATE POLICY alert_events_tenant_policy ON alert_events USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);
