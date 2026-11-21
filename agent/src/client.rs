use anyhow::{anyhow, bail, Context, Result};
use reqwest::Client;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;

use crate::capabilities::Capabilities;
use crate::proto_num::{deserialize_i32, deserialize_i64};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct NodeState {
    pub node_id: String,
    pub node_token: String,
    pub organization_id: String,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DesiredWorkload {
    pub id: String,
    #[serde(default)]
    pub project_id: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub image: String,
    #[serde(default)]
    pub env_vars: HashMap<String, String>,
    #[serde(default, deserialize_with = "deserialize_i32")]
    pub cpu_shares: i32,
    #[serde(default, deserialize_with = "deserialize_i64")]
    pub memory_limit_mb: i64,
    #[serde(default, deserialize_with = "deserialize_i32")]
    pub host_port: i32,
    #[serde(default, deserialize_with = "deserialize_i32")]
    pub container_port: i32,
    #[serde(default, deserialize_with = "deserialize_i64")]
    pub desired_generation: i64,
    #[serde(default)]
    pub action: String,
    /// container | docker-compose | microvm | wasm | static-site
    #[serde(default)]
    pub runtime: String,
    #[serde(default)]
    pub network_name: String,
    #[serde(default)]
    pub network_alias: String,
    /// rolling | recreate (#15)
    #[serde(default)]
    pub update_strategy: String,
    #[serde(default, deserialize_with = "deserialize_i32")]
    pub surge_host_port: i32,
    #[serde(default)]
    pub readiness_path: String,
    #[serde(default)]
    pub probe_spec_json: String,
    /// JSON array of {name,mount_path,read_only} (#16).
    #[serde(default)]
    pub volume_mounts_json: String,
    /// Pending backup|delete JSON (#16).
    #[serde(default)]
    pub volume_pending_op_json: String,
    /// Parent workload id for stable Docker volume names.
    #[serde(default)]
    pub workload_id: String,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ObservedWorkload {
    pub id: String,
    pub status: String,
    pub container_id: String,
    pub observed_generation: i64,
    pub message: String,
    pub host_port: i32,
    pub log_lines: Vec<String>,
    #[serde(default)]
    pub ready: bool,
    #[serde(default)]
    pub probe_status_json: String,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct EnrollRequest<'a> {
    join_token: &'a str,
    node_name: &'a str,
    capabilities: &'a Capabilities,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct EnrollResponse {
    node_id: String,
    node_token: String,
    organization_id: String,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct HeartbeatRequest<'a> {
    node_id: &'a str,
    cpu_usage_percent: f64,
    memory_free_mb: i64,
    status: &'a str,
    capabilities: &'a Capabilities,
    #[serde(default)]
    running_workload_ids: Vec<String>,
    #[serde(default, skip_serializing_if = "slice_is_empty")]
    observed_managed_databases: &'a [ObservedManagedDatabase],
}

fn slice_is_empty<T>(value: &&[T]) -> bool {
    value.is_empty()
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct DesiredManagedDatabase {
    pub id: String,
    #[allow(dead_code)]
    #[serde(default)]
    pub project_id: String,
    #[allow(dead_code)]
    #[serde(default)]
    pub name: String,
    #[allow(dead_code)]
    #[serde(default)]
    pub engine: String,
    #[serde(default)]
    pub image: String,
    #[serde(default)]
    pub env_vars: HashMap<String, String>,
    #[serde(default)]
    pub command: Vec<String>,
    #[serde(default, deserialize_with = "deserialize_i32")]
    pub host_port: i32,
    #[serde(default, deserialize_with = "deserialize_i32")]
    pub container_port: i32,
    #[serde(default)]
    pub volume_name: String,
    #[serde(default)]
    pub volume_mount_path: String,
    #[serde(default)]
    pub action: String,
}

#[derive(Debug, Clone, Serialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct ObservedManagedDatabase {
    pub id: String,
    pub status: String,
    pub container_id: String,
    pub message: String,
    pub host_port: i32,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DesiredBuild {
    pub id: String,
    #[allow(dead_code)]
    #[serde(default)]
    pub workload_id: String,
    #[serde(default)]
    pub git_url: String,
    #[serde(default)]
    pub git_ref: String,
    #[serde(default)]
    pub dockerfile_path: String,
    #[serde(default)]
    pub image_tag: String,
    #[serde(default)]
    pub build_args: HashMap<String, String>,
    #[serde(default)]
    pub registry: String,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ObservedBuild {
    pub id: String,
    pub status: String,
    pub image: String,
    pub message: String,
    pub log_lines: Vec<String>,
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct DesiredEdgeRoute {
    #[serde(default)]
    pub domain_id: String,
    #[serde(default)]
    pub hostname: String,
    #[serde(default)]
    pub upstream: String,
    #[serde(default)]
    pub tls_mode: String,
    #[allow(dead_code)]
    #[serde(default, deserialize_with = "deserialize_i64")]
    pub edge_generation: i64,
    #[serde(default)]
    pub action: String,
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct DesiredEdgePlan {
    #[serde(default, deserialize_with = "deserialize_i64")]
    pub generation: i64,
    #[serde(default)]
    pub routes: Vec<DesiredEdgeRoute>,
    #[serde(default)]
    pub acme_ca: String,
    #[serde(default)]
    pub caddy_admin_url: String,
}

#[derive(Debug, Clone, Serialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct ObservedEdge {
    pub generation: i64,
    pub status: String,
    pub message: String,
    pub log_lines: Vec<String>,
    pub route_statuses: HashMap<String, String>,
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct DesiredNetworkPeer {
    #[allow(dead_code)]
    #[serde(default)]
    pub node_id: String,
    #[allow(dead_code)]
    #[serde(default)]
    pub node_name: String,
    #[allow(dead_code)]
    #[serde(default)]
    pub public_key: String,
    #[allow(dead_code)]
    #[serde(default)]
    pub address: String,
    #[allow(dead_code)]
    #[serde(default)]
    pub endpoint: String,
    #[allow(dead_code)]
    #[serde(default)]
    pub allowed_ips: String,
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct DesiredNetworkDNSRecord {
    #[serde(default)]
    pub name: String,
    #[serde(default, rename = "type")]
    pub r#type: String,
    #[serde(default)]
    pub value: String,
    #[allow(dead_code)]
    #[serde(default, deserialize_with = "deserialize_i32")]
    pub ttl_seconds: i32,
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct DesiredNetworkCertificate {
    #[serde(default)]
    pub hostname: String,
    #[serde(default)]
    pub cert_pem: String,
    #[serde(default)]
    pub key_pem: String,
    #[allow(dead_code)]
    #[serde(default)]
    pub not_after: String,
    #[allow(dead_code)]
    #[serde(default)]
    pub issuer: String,
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct DesiredNetworkPlan {
    #[serde(default, deserialize_with = "deserialize_i64")]
    pub generation: i64,
    #[allow(dead_code)]
    #[serde(default)]
    pub interface: String,
    #[serde(default)]
    pub address: String,
    #[allow(dead_code)]
    #[serde(default, deserialize_with = "deserialize_i32")]
    pub listen_port: i32,
    #[allow(dead_code)]
    #[serde(default)]
    pub private_key: String,
    #[allow(dead_code)]
    #[serde(default)]
    pub public_key: String,
    #[serde(default)]
    pub peers: Vec<DesiredNetworkPeer>,
    #[serde(default)]
    pub dns_records: Vec<DesiredNetworkDNSRecord>,
    #[serde(default)]
    pub nftables: String,
    #[serde(default)]
    pub wg_config: String,
    #[serde(default)]
    pub certificates: Vec<DesiredNetworkCertificate>,
    #[serde(default)]
    pub apply: bool,
}

#[derive(Debug, Clone, Serialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct ObservedNetwork {
    pub generation: i64,
    pub status: String,
    pub message: String,
    pub log_lines: Vec<String>,
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct DesiredWasmFunction {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    #[allow(dead_code)]
    pub project_id: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub image_ref: String,
    #[serde(default, deserialize_with = "deserialize_i64")]
    pub fuel_limit: i64,
    #[serde(default, deserialize_with = "deserialize_i32")]
    pub memory_limit_mb: i32,
    #[serde(default, deserialize_with = "deserialize_i32")]
    pub listen_port: i32,
    #[serde(default)]
    pub hostname: String,
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct DesiredWasmPlan {
    #[serde(default, deserialize_with = "deserialize_i64")]
    pub generation: i64,
    #[serde(default)]
    pub functions: Vec<DesiredWasmFunction>,
}

#[derive(Debug, Clone, Serialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct ObservedWasmFunction {
    pub id: String,
    pub status: String,
    pub message: String,
    pub artifact_path: String,
}

#[derive(Debug, Clone, Serialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct ObservedWasm {
    pub generation: i64,
    pub status: String,
    pub message: String,
    pub log_lines: Vec<String>,
    pub functions: Vec<ObservedWasmFunction>,
}

/// Compose stack applied as a unit (#143).
#[derive(Debug, Clone, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct DesiredComposeStack {
    pub id: String,
    #[serde(default)]
    pub project_id: String,
    #[allow(dead_code)]
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub git_url: String,
    #[serde(default)]
    pub git_ref: String,
    #[serde(default)]
    pub compose_path: String,
    #[serde(default)]
    pub compose_project: String,
    /// up | stop | down
    #[serde(default)]
    pub action: String,
    #[serde(default, deserialize_with = "deserialize_i64")]
    pub generation: i64,
    #[serde(default)]
    pub public_services: Vec<String>,
    /// Decrypted by the control plane for this node only. Never log the values.
    #[serde(default)]
    pub env: HashMap<String, String>,
    #[serde(default)]
    pub domains: HashMap<String, String>,
    #[serde(default)]
    pub purge_volumes: bool,
}

#[derive(Debug, Clone, Serialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct ObservedComposeService {
    pub name: String,
    pub state: String,
    pub health: String,
    pub exit_code: i32,
    pub image: String,
    pub ports: String,
    #[serde(default)]
    pub container_port: i32,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct ComposeVolume {
    pub name: String,
    pub kind: String,
    pub source: String,
}

#[derive(Debug, Clone, Serialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct ObservedComposeLogLine {
    pub service: String,
    pub line: String,
}

#[derive(Debug, Clone, Serialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct ObservedComposeStack {
    pub id: String,
    pub status: String,
    pub observed_generation: i64,
    pub message: String,
    pub services: Vec<ObservedComposeService>,
    pub log_lines: Vec<ObservedComposeLogLine>,
    #[serde(default)]
    pub generated_env: HashMap<String, String>,
    #[serde(default)]
    pub volumes: Vec<ComposeVolume>,
}

#[derive(Debug, Clone, Default)]
pub struct HeartbeatResult {
    pub desired_workloads: Vec<DesiredWorkload>,
    pub desired_builds: Vec<DesiredBuild>,
    pub desired_edge: Option<DesiredEdgePlan>,
    pub desired_network: Option<DesiredNetworkPlan>,
    pub desired_wasm: Option<DesiredWasmPlan>,
    pub desired_agent_version: Option<String>,
    /// One-shot replacement bearer after UI/API RotateNodeToken (#13).
    pub rotated_node_token: Option<String>,
    pub desired_managed_databases: Vec<DesiredManagedDatabase>,
    pub desired_compose_stacks: Vec<DesiredComposeStack>,
}

#[derive(Debug, Clone, Default)]
pub struct WatchDesiredResult {
    pub fingerprint: String,
    pub changed: bool,
    pub timed_out: bool,
    pub desired: HeartbeatResult,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct WatchDesiredResponse {
    #[serde(default)]
    fingerprint: String,
    #[serde(default)]
    changed: bool,
    #[serde(default)]
    timed_out: bool,
    #[serde(default)]
    desired_workloads: Vec<DesiredWorkload>,
    #[serde(default)]
    desired_builds: Vec<DesiredBuild>,
    #[serde(default)]
    desired_edge: Option<DesiredEdgePlan>,
    #[serde(default)]
    desired_network: Option<DesiredNetworkPlan>,
    #[serde(default)]
    desired_wasm: Option<DesiredWasmPlan>,
    #[serde(default)]
    desired_agent_version: Option<String>,
    #[serde(default)]
    desired_managed_databases: Vec<DesiredManagedDatabase>,
    #[serde(default)]
    desired_compose_stacks: Vec<DesiredComposeStack>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct HeartbeatResponse {
    #[serde(default)]
    #[allow(dead_code)]
    node_id: String,
    #[serde(default)]
    desired_workloads: Vec<DesiredWorkload>,
    #[serde(default)]
    desired_builds: Vec<DesiredBuild>,
    #[serde(default)]
    desired_edge: Option<DesiredEdgePlan>,
    #[serde(default)]
    desired_network: Option<DesiredNetworkPlan>,
    #[serde(default)]
    desired_wasm: Option<DesiredWasmPlan>,
    #[serde(default)]
    desired_agent_version: Option<String>,
    #[serde(default)]
    rotated_node_token: Option<String>,
    #[serde(default)]
    desired_managed_databases: Vec<DesiredManagedDatabase>,
    #[serde(default)]
    desired_compose_stacks: Vec<DesiredComposeStack>,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct ReportObservedRequest<'a> {
    node_id: &'a str,
    workloads: &'a [ObservedWorkload],
}

#[derive(Clone)]
pub struct AgentClient {
    http: Client,
    api_base: String,
}

impl AgentClient {
    pub fn new(api_base: String) -> Result<Self> {
        let mut builder = Client::builder()
            .user_agent(format!("norix-agent/{}", env!("CARGO_PKG_VERSION")))
            .timeout(std::time::Duration::from_secs(60));
        // Optional mTLS identity (#13): NORIX_AGENT_CLIENT_CERT + NORIX_AGENT_CLIENT_KEY
        // (+ optional NORIX_AGENT_CA_CERT to trust the control-plane / CA).
        if let (Ok(cert_path), Ok(key_path)) = (
            std::env::var("NORIX_AGENT_CLIENT_CERT"),
            std::env::var("NORIX_AGENT_CLIENT_KEY"),
        ) {
            let cert_pem = std::fs::read(&cert_path)
                .with_context(|| format!("read NORIX_AGENT_CLIENT_CERT {cert_path}"))?;
            let key_pem = std::fs::read(&key_path)
                .with_context(|| format!("read NORIX_AGENT_CLIENT_KEY {key_path}"))?;
            let identity = reqwest::Identity::from_pem(&[cert_pem, key_pem].concat())
                .context("parse agent client TLS identity (PEM cert+key)")?;
            builder = builder.identity(identity);
            tracing::info!("agent mTLS client identity loaded");
        }
        if let Ok(ca_path) = std::env::var("NORIX_AGENT_CA_CERT") {
            let ca_pem = std::fs::read(&ca_path)
                .with_context(|| format!("read NORIX_AGENT_CA_CERT {ca_path}"))?;
            let ca =
                reqwest::Certificate::from_pem(&ca_pem).context("parse NORIX_AGENT_CA_CERT PEM")?;
            builder = builder.add_root_certificate(ca);
        }
        let http = builder.build().context("build http client")?;
        Ok(Self { http, api_base })
    }

    pub async fn enroll(
        &self,
        join_token: &str,
        node_name: &str,
        capabilities: &Capabilities,
    ) -> Result<NodeState> {
        let body = EnrollRequest {
            join_token,
            node_name,
            capabilities,
        };
        let response = self
            .post_json("/v1.AgentService/EnrollNode", None, &body)
            .await?;
        let parsed: EnrollResponse =
            serde_json::from_value(response).context("decode enroll response")?;
        Ok(NodeState {
            node_id: parsed.node_id,
            node_token: parsed.node_token,
            organization_id: parsed.organization_id,
        })
    }

    pub async fn heartbeat(
        &self,
        state: &NodeState,
        capabilities: &Capabilities,
        memory_free_mb: i64,
        cpu_usage_percent: f64,
        running_workload_ids: Vec<String>,
        observed_managed_databases: &[ObservedManagedDatabase],
    ) -> Result<HeartbeatResult> {
        let body = HeartbeatRequest {
            node_id: &state.node_id,
            cpu_usage_percent,
            memory_free_mb,
            status: "online",
            capabilities,
            running_workload_ids,
            observed_managed_databases,
        };
        let response = self
            .post_json("/v1.AgentService/Heartbeat", Some(&state.node_token), &body)
            .await?;
        match serde_json::from_value::<HeartbeatResponse>(response.clone()) {
            Ok(parsed) => Ok(HeartbeatResult {
                desired_workloads: parsed.desired_workloads,
                desired_builds: parsed.desired_builds,
                desired_edge: parsed.desired_edge,
                desired_network: parsed.desired_network.filter(|plan| plan.generation > 0),
                desired_wasm: parsed
                    .desired_wasm
                    .filter(|plan| !plan.functions.is_empty()),
                desired_agent_version: parsed
                    .desired_agent_version
                    .map(|v| v.trim().to_string())
                    .filter(|v| !v.is_empty()),
                rotated_node_token: parsed
                    .rotated_node_token
                    .map(|v| v.trim().to_string())
                    .filter(|v| !v.is_empty()),
                desired_managed_databases: parsed.desired_managed_databases,
                desired_compose_stacks: parsed.desired_compose_stacks,
            }),
            Err(error) => {
                let mut result = HeartbeatResult::default();
                if let Some(items) = response
                    .get("desiredWorkloads")
                    .or_else(|| response.get("desired_workloads"))
                    .and_then(|value| value.as_array())
                {
                    for item in items {
                        match serde_json::from_value::<DesiredWorkload>(item.clone()) {
                            Ok(workload) => result.desired_workloads.push(workload),
                            Err(item_error) => {
                                tracing::warn!(%item_error, item = %item, "skip undecodable desired workload");
                            }
                        }
                    }
                }
                if let Some(items) = response
                    .get("desiredBuilds")
                    .or_else(|| response.get("desired_builds"))
                    .and_then(|value| value.as_array())
                {
                    for item in items {
                        match serde_json::from_value::<DesiredBuild>(item.clone()) {
                            Ok(build) => result.desired_builds.push(build),
                            Err(item_error) => {
                                tracing::warn!(%item_error, item = %item, "skip undecodable desired build");
                            }
                        }
                    }
                }
                if let Some(edge) = response
                    .get("desiredEdge")
                    .or_else(|| response.get("desired_edge"))
                {
                    if let Ok(parsed) = serde_json::from_value::<DesiredEdgePlan>(edge.clone()) {
                        if parsed.generation > 0 {
                            result.desired_edge = Some(parsed);
                        }
                    }
                }
                if let Some(network) = response
                    .get("desiredNetwork")
                    .or_else(|| response.get("desired_network"))
                {
                    if let Ok(parsed) =
                        serde_json::from_value::<DesiredNetworkPlan>(network.clone())
                    {
                        if parsed.generation > 0 {
                            result.desired_network = Some(parsed);
                        }
                    }
                }
                if let Some(wasm) = response
                    .get("desiredWasm")
                    .or_else(|| response.get("desired_wasm"))
                {
                    if let Ok(parsed) = serde_json::from_value::<DesiredWasmPlan>(wasm.clone()) {
                        if !parsed.functions.is_empty() {
                            result.desired_wasm = Some(parsed);
                        }
                    }
                }
                if let Some(items) = response
                    .get("desiredComposeStacks")
                    .or_else(|| response.get("desired_compose_stacks"))
                    .and_then(|value| value.as_array())
                {
                    for item in items {
                        match serde_json::from_value::<DesiredComposeStack>(item.clone()) {
                            Ok(stack) => result.desired_compose_stacks.push(stack),
                            Err(item_error) => {
                                tracing::warn!(%item_error, "skip undecodable desired compose stack");
                            }
                        }
                    }
                }
                if result.desired_workloads.is_empty()
                    && result.desired_builds.is_empty()
                    && result.desired_edge.is_none()
                    && result.desired_network.is_none()
                    && result.desired_wasm.is_none()
                {
                    tracing::warn!(%error, body = %response, "heartbeat decode partial; treating desired as empty");
                }
                Ok(result)
            }
        }
    }

    /// Long-poll desired state (#44). Blocks until fingerprint changes or timeout.
    pub async fn watch_desired(
        &self,
        state: &NodeState,
        known_fingerprint: &str,
        timeout_seconds: u64,
    ) -> Result<WatchDesiredResult> {
        #[derive(Serialize)]
        #[serde(rename_all = "camelCase")]
        struct Body<'a> {
            node_id: &'a str,
            known_fingerprint: &'a str,
            timeout_seconds: i32,
        }
        let timeout_seconds = timeout_seconds.clamp(1, 30) as i32;
        let body = Body {
            node_id: &state.node_id,
            known_fingerprint,
            timeout_seconds,
        };
        // Request budget: server wait + small network cushion (client default is 60s).
        let response = self
            .post_json(
                "/v1.AgentService/WatchDesired",
                Some(&state.node_token),
                &body,
            )
            .await?;
        let parsed: WatchDesiredResponse =
            serde_json::from_value(response).context("decode WatchDesired response")?;
        Ok(WatchDesiredResult {
            fingerprint: parsed.fingerprint,
            changed: parsed.changed,
            timed_out: parsed.timed_out,
            desired: HeartbeatResult {
                desired_workloads: parsed.desired_workloads,
                desired_builds: parsed.desired_builds,
                desired_edge: parsed.desired_edge,
                desired_network: parsed.desired_network.filter(|plan| plan.generation > 0),
                desired_wasm: parsed
                    .desired_wasm
                    .filter(|plan| !plan.functions.is_empty()),
                desired_agent_version: parsed
                    .desired_agent_version
                    .map(|v| v.trim().to_string())
                    .filter(|v| !v.is_empty()),
                rotated_node_token: None,
                desired_managed_databases: parsed.desired_managed_databases,
                desired_compose_stacks: parsed.desired_compose_stacks,
            },
        })
    }

    pub async fn report_wasm(&self, state: &NodeState, wasm: &ObservedWasm) -> Result<()> {
        #[derive(Serialize)]
        #[serde(rename_all = "camelCase")]
        struct Body<'a> {
            node_id: &'a str,
            wasm: &'a ObservedWasm,
        }
        let body = Body {
            node_id: &state.node_id,
            wasm,
        };
        self.post_json(
            "/v1.AgentService/ReportWasm",
            Some(&state.node_token),
            &body,
        )
        .await?;
        Ok(())
    }

    pub async fn report_edge(&self, state: &NodeState, edge: &ObservedEdge) -> Result<()> {
        #[derive(Serialize)]
        #[serde(rename_all = "camelCase")]
        struct Body<'a> {
            node_id: &'a str,
            edge: &'a ObservedEdge,
        }
        let body = Body {
            node_id: &state.node_id,
            edge,
        };
        self.post_json(
            "/v1.AgentService/ReportEdge",
            Some(&state.node_token),
            &body,
        )
        .await?;
        Ok(())
    }

    pub async fn report_network(&self, state: &NodeState, network: &ObservedNetwork) -> Result<()> {
        #[derive(Serialize)]
        #[serde(rename_all = "camelCase")]
        struct Body<'a> {
            node_id: &'a str,
            network: &'a ObservedNetwork,
        }
        let body = Body {
            node_id: &state.node_id,
            network,
        };
        self.post_json(
            "/v1.AgentService/ReportNetwork",
            Some(&state.node_token),
            &body,
        )
        .await?;
        Ok(())
    }

    pub async fn report_observed(
        &self,
        state: &NodeState,
        workloads: &[ObservedWorkload],
    ) -> Result<()> {
        if workloads.is_empty() {
            return Ok(());
        }
        let body = ReportObservedRequest {
            node_id: &state.node_id,
            workloads,
        };
        self.post_json(
            "/v1.AgentService/ReportObserved",
            Some(&state.node_token),
            &body,
        )
        .await?;
        Ok(())
    }

    pub async fn report_compose_stack(
        &self,
        state: &NodeState,
        stack: &ObservedComposeStack,
    ) -> Result<()> {
        #[derive(Serialize)]
        #[serde(rename_all = "camelCase")]
        struct Body<'a> {
            node_id: &'a str,
            stack: &'a ObservedComposeStack,
        }
        let body = Body {
            node_id: &state.node_id,
            stack,
        };
        self.post_json(
            "/v1.AgentService/ReportComposeStack",
            Some(&state.node_token),
            &body,
        )
        .await?;
        Ok(())
    }

    pub async fn report_build(&self, state: &NodeState, build: &ObservedBuild) -> Result<()> {
        #[derive(Serialize)]
        #[serde(rename_all = "camelCase")]
        struct Body<'a> {
            node_id: &'a str,
            build: &'a ObservedBuild,
        }
        let body = Body {
            node_id: &state.node_id,
            build,
        };
        self.post_json(
            "/v1.AgentService/ReportBuild",
            Some(&state.node_token),
            &body,
        )
        .await?;
        Ok(())
    }

    async fn post_json<T: Serialize>(
        &self,
        procedure: &str,
        bearer: Option<&str>,
        body: &T,
    ) -> Result<Value> {
        let url = format!("{}{}", self.api_base, procedure);
        let mut request = self
            .http
            .post(url)
            .header("Content-Type", "application/json")
            .header("Connect-Protocol-Version", "1")
            .json(body);
        if let Some(token) = bearer {
            request = request.bearer_auth(token);
        }
        let response = request.send().await.context("connect request failed")?;
        let status = response.status();
        let payload = response
            .json::<Value>()
            .await
            .context("decode connect response")?;
        if !status.is_success() {
            let message = payload
                .get("message")
                .and_then(Value::as_str)
                .or_else(|| payload.get("error").and_then(Value::as_str))
                .unwrap_or("request failed");
            bail!("connect {procedure} failed ({status}): {message}");
        }
        if payload.get("code").is_some() && payload.get("message").is_some() {
            return Err(anyhow!(
                "connect {procedure} error: {}",
                payload
                    .get("message")
                    .and_then(Value::as_str)
                    .unwrap_or("unknown")
            ));
        }
        Ok(payload)
    }
}
