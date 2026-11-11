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
    #[allow(dead_code)]
    pub name: String,
    #[serde(default)]
    pub image: String,
    #[serde(default)]
    pub env_vars: HashMap<String, String>,
    #[serde(default, deserialize_with = "deserialize_i32")]
    #[allow(dead_code)]
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
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct HeartbeatResponse {
    #[serde(default)]
    #[allow(dead_code)]
    node_id: String,
    #[serde(default)]
    desired_workloads: Vec<DesiredWorkload>,
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
        let http = Client::builder()
            .user_agent(format!("norix-agent/{}", env!("CARGO_PKG_VERSION")))
            .timeout(std::time::Duration::from_secs(60))
            .build()
            .context("build http client")?;
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
    ) -> Result<Vec<DesiredWorkload>> {
        let body = HeartbeatRequest {
            node_id: &state.node_id,
            cpu_usage_percent,
            memory_free_mb,
            status: "online",
            capabilities,
            running_workload_ids,
        };
        let response = self
            .post_json(
                "/v1.AgentService/Heartbeat",
                Some(&state.node_token),
                &body,
            )
            .await?;
        // Heartbeat is presence-critical: never treat a successful Connect response as a
        // reconnect failure just because desired_workloads JSON is awkward (int64 strings, etc).
        match serde_json::from_value::<HeartbeatResponse>(response.clone()) {
            Ok(parsed) => Ok(parsed.desired_workloads),
            Err(error) => {
                if let Some(items) = response
                    .get("desiredWorkloads")
                    .or_else(|| response.get("desired_workloads"))
                    .and_then(|value| value.as_array())
                {
                    let mut desired = Vec::with_capacity(items.len());
                    for item in items {
                        match serde_json::from_value::<DesiredWorkload>(item.clone()) {
                            Ok(workload) => desired.push(workload),
                            Err(item_error) => {
                                tracing::warn!(%item_error, item = %item, "skip undecodable desired workload");
                            }
                        }
                    }
                    return Ok(desired);
                }
                tracing::warn!(%error, body = %response, "heartbeat decode partial; treating desired as empty");
                Ok(Vec::new())
            }
        }
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
