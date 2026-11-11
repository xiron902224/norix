use anyhow::{bail, Context, Result};
use reqwest::Client;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::collections::HashMap;
use std::path::PathBuf;
use std::time::Duration;
use tracing::{info, warn};

use crate::client::ObservedEdge;
use crate::client::DesiredEdgePlan;

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
struct EdgeState {
    last_generation: i64,
    last_config: String,
}

pub async fn apply_edge(plan: &DesiredEdgePlan) -> ObservedEdge {
    let mut logs = vec![format!(
        "edge generation {} routes={}",
        plan.generation,
        plan.routes.len()
    )];
    let admin = if plan.caddy_admin_url.is_empty() {
        "http://127.0.0.1:2019".to_string()
    } else {
        plan.caddy_admin_url.trim_end_matches('/').to_string()
    };
    let client = Client::builder()
        .timeout(Duration::from_secs(10))
        .build()
        .unwrap_or_else(|_| Client::new());

    if let Err(error) = ping_caddy(&client, &admin).await {
        return ObservedEdge {
            generation: plan.generation,
            status: "failed".into(),
            message: format!("caddy admin unreachable at {admin}: {error}"),
            log_lines: {
                logs.push(error.to_string());
                logs
            },
            route_statuses: HashMap::new(),
        };
    }

    let state_path = edge_state_path();
    let previous = read_state(&state_path);
    let previous_config = fetch_config(&client, &admin)
        .await
        .unwrap_or(previous.last_config.clone());

    let (config, mut route_statuses, build_logs) = match build_config(plan) {
        Ok(value) => value,
        Err(error) => {
            return ObservedEdge {
                generation: plan.generation,
                status: "failed".into(),
                message: error.to_string(),
                log_lines: {
                    logs.push(error.to_string());
                    logs
                },
                route_statuses: HashMap::new(),
            };
        }
    };
    logs.extend(build_logs);

    if let Err(error) = load_config(&client, &admin, &config).await {
        logs.push(format!("load failed: {error}"));
        if !previous_config.is_empty() {
            let _ = load_config(&client, &admin, &previous_config).await;
            logs.push("rolled back previous caddy config".into());
            return ObservedEdge {
                generation: plan.generation,
                status: "rolled_back".into(),
                message: error.to_string(),
                log_lines: logs,
                route_statuses: fail_all(plan),
            };
        }
        return ObservedEdge {
            generation: plan.generation,
            status: "failed".into(),
            message: error.to_string(),
            log_lines: logs,
            route_statuses: fail_all(plan),
        };
    }

    for route in &plan.routes {
        if route.action != "upsert" || route.upstream.is_empty() {
            continue;
        }
        let health_url = route.upstream.replace("host.docker.internal", "127.0.0.1");
        if let Err(error) = health_check(&client, &health_url).await {
            warn!(hostname = %route.hostname, %error, "edge health check failed");
            logs.push(format!(
                "health check failed for {} → {}: {error}",
                route.hostname, route.upstream
            ));
            if !previous_config.is_empty() {
                let _ = load_config(&client, &admin, &previous_config).await;
                logs.push("rolled back previous caddy config after health failure".into());
                return ObservedEdge {
                    generation: plan.generation,
                    status: "rolled_back".into(),
                    message: error.to_string(),
                    log_lines: logs,
                    route_statuses: fail_all(plan),
                };
            }
            route_statuses.insert(route.domain_id.clone(), "failed".into());
            return ObservedEdge {
                generation: plan.generation,
                status: "failed".into(),
                message: error.to_string(),
                log_lines: logs,
                route_statuses,
            };
        }
        route_statuses.insert(route.domain_id.clone(), "active".into());
        logs.push(format!("healthy {} → {}", route.hostname, route.upstream));
    }

    write_state(
        &state_path,
        &EdgeState {
            last_generation: plan.generation,
            last_config: config,
        },
    );
    info!(generation = plan.generation, "edge config active");
    logs.push("edge config active".into());
    ObservedEdge {
        generation: plan.generation,
        status: "active".into(),
        message: String::new(),
        log_lines: logs,
        route_statuses,
    }
}

fn build_config(plan: &DesiredEdgePlan) -> Result<(String, HashMap<String, String>, Vec<String>)> {
    let mut logs = Vec::new();
    let mut route_statuses = HashMap::new();
    let mut routes: Vec<Value> = Vec::new();
    let mut internal_hosts = Vec::new();
    let mut acme_hosts = Vec::new();

    for route in &plan.routes {
        if route.action == "delete" {
            route_statuses.insert(route.domain_id.clone(), "deleted".into());
            logs.push(format!("delete {}", route.hostname));
            continue;
        }
        if route.upstream.is_empty() {
            route_statuses.insert(route.domain_id.clone(), "failed".into());
            logs.push(format!("skip {} (no upstream)", route.hostname));
            continue;
        }
        let dial = route
            .upstream
            .trim_start_matches("http://")
            .trim_start_matches("https://")
            .to_string();
        routes.push(json!({
            "match": [{"host": [route.hostname]}],
            "handle": [{
                "handler": "reverse_proxy",
                "upstreams": [{"dial": dial}]
            }]
        }));
        if route.tls_mode == "acme" {
            acme_hosts.push(route.hostname.clone());
        } else {
            internal_hosts.push(route.hostname.clone());
        }
        route_statuses.insert(route.domain_id.clone(), "provisioning".into());
        logs.push(format!(
            "route {} tls={} upstream={}",
            route.hostname, route.tls_mode, route.upstream
        ));
    }
    routes.push(json!({
        "handle": [{
            "handler": "static_response",
            "body": "norix edge",
            "status_code": 200
        }]
    }));

    let mut apps = json!({
        "http": {
            "servers": {
                "norix": {
                    "listen": [":80", ":443"],
                    "routes": routes
                }
            }
        }
    });

    let mut policies = Vec::new();
    if !internal_hosts.is_empty() {
        policies.push(json!({
            "subjects": internal_hosts,
            "issuers": [{"module": "internal"}]
        }));
    }
    if !acme_hosts.is_empty() {
        let mut issuer = json!({"module": "acme"});
        if !plan.acme_ca.is_empty() {
            issuer["ca"] = json!(plan.acme_ca);
        }
        policies.push(json!({
            "subjects": acme_hosts,
            "issuers": [issuer]
        }));
    }
    if !policies.is_empty() {
        apps["tls"] = json!({"automation": {"policies": policies}});
    }

    let payload = json!({"apps": apps});
    Ok((payload.to_string(), route_statuses, logs))
}

fn fail_all(plan: &DesiredEdgePlan) -> HashMap<String, String> {
    plan.routes
        .iter()
        .map(|route| (route.domain_id.clone(), "failed".into()))
        .collect()
}

async fn ping_caddy(client: &Client, admin: &str) -> Result<()> {
    let response = client
        .get(format!("{admin}/config/"))
        .send()
        .await
        .context("caddy ping")?;
    if !response.status().is_success() {
        bail!("status {}", response.status());
    }
    Ok(())
}

async fn fetch_config(client: &Client, admin: &str) -> Result<String> {
    let response = client
        .get(format!("{admin}/config/"))
        .send()
        .await
        .context("fetch caddy config")?;
    let status = response.status();
    let body = response.text().await.unwrap_or_default();
    if !status.is_success() {
        bail!("status {status}");
    }
    Ok(body)
}

async fn load_config(client: &Client, admin: &str, config: &str) -> Result<()> {
    let response = client
        .post(format!("{admin}/load"))
        .header("Content-Type", "application/json")
        .body(config.to_string())
        .send()
        .await
        .context("load caddy config")?;
    let status = response.status();
    let body = response.text().await.unwrap_or_default();
    if !status.is_success() {
        bail!("load status {status}: {}", body.trim());
    }
    Ok(())
}

async fn health_check(client: &Client, upstream: &str) -> Result<()> {
    let response = client
        .get(upstream)
        .timeout(Duration::from_secs(3))
        .send()
        .await
        .context("health check")?;
    if response.status().as_u16() >= 500 {
        bail!("upstream status {}", response.status());
    }
    Ok(())
}

fn edge_state_path() -> PathBuf {
    let dir = std::env::var("NORIX_EDGE_STATE_DIR")
        .unwrap_or_else(|_| std::env::temp_dir().join("norix-edge").display().to_string());
    let path = PathBuf::from(dir);
    let _ = std::fs::create_dir_all(&path);
    path.join("state.json")
}

fn read_state(path: &PathBuf) -> EdgeState {
    std::fs::read_to_string(path)
        .ok()
        .and_then(|raw| serde_json::from_str(&raw).ok())
        .unwrap_or_default()
}

fn write_state(path: &PathBuf, state: &EdgeState) {
    if let Ok(raw) = serde_json::to_string_pretty(state) {
        let _ = std::fs::write(path, raw);
    }
}
