use anyhow::{bail, Context, Result};
use serde::{Deserialize, Serialize};
use serde_json::json;
use std::time::Duration;
use tokio::net::TcpStream;
use tokio::process::Command;
use tokio::time::timeout;

use crate::client::DesiredWorkload;

#[derive(Debug, Clone, Deserialize, Serialize, Default)]
pub struct ProbeConfig {
    #[serde(default)]
    pub r#type: String,
    #[serde(default)]
    pub path: String,
    #[serde(default)]
    pub port: i32,
    #[serde(default)]
    pub command: Vec<String>,
    #[serde(default)]
    pub initial_delay_seconds: i32,
    #[serde(default)]
    pub period_seconds: i32,
    #[serde(default)]
    pub timeout_seconds: i32,
    #[serde(default)]
    pub failure_threshold: i32,
}

#[derive(Debug, Clone, Deserialize, Serialize, Default)]
pub struct ProbeSpec {
    #[serde(default)]
    pub readiness: ProbeConfig,
    #[serde(default)]
    pub liveness: ProbeConfig,
}

#[derive(Debug, Clone, Serialize, Default)]
pub struct ProbeStatus {
    pub readiness: String,
    pub liveness: String,
    pub message: String,
}

pub fn parse_probe_spec(raw: &str) -> ProbeSpec {
    let raw = raw.trim();
    if raw.is_empty() || raw == "{}" {
        return ProbeSpec {
            readiness: ProbeConfig {
                r#type: "http".into(),
                path: "/".into(),
                period_seconds: 10,
                timeout_seconds: 3,
                failure_threshold: 3,
                ..Default::default()
            },
            liveness: ProbeConfig {
                r#type: "none".into(),
                ..Default::default()
            },
        };
    }
    serde_json::from_str(raw).unwrap_or_default()
}

pub async fn evaluate_probes(
    desired: &DesiredWorkload,
    container_name: &str,
) -> (bool, ProbeStatus) {
    let spec = if desired.probe_spec_json.trim().is_empty() {
        ProbeSpec {
            readiness: ProbeConfig {
                r#type: if desired.readiness_path.is_empty() {
                    "tcp".into()
                } else {
                    "http".into()
                },
                path: if desired.readiness_path.is_empty() {
                    "/".into()
                } else {
                    desired.readiness_path.clone()
                },
                ..Default::default()
            },
            ..Default::default()
        }
    } else {
        parse_probe_spec(&desired.probe_spec_json)
    };

    let mut status = ProbeStatus {
        readiness: "none".into(),
        liveness: "none".into(),
        message: String::new(),
    };

    let ready = match run_probe(&spec.readiness, desired, container_name).await {
        Ok(()) => {
            status.readiness = "pass".into();
            true
        }
        Err(err) => {
            status.readiness = "fail".into();
            status.message = format!("readiness: {err}");
            false
        }
    };

    if spec.liveness.r#type.trim().is_empty()
        || spec.liveness.r#type.eq_ignore_ascii_case("none")
        || spec.liveness.r#type.eq_ignore_ascii_case("off")
    {
        status.liveness = "none".into();
    } else if let Err(err) = run_probe(&spec.liveness, desired, container_name).await {
        status.liveness = "fail".into();
        if status.message.is_empty() {
            status.message = format!("liveness: {err}");
        } else {
            status.message = format!("{}; liveness: {err}", status.message);
        }
    } else {
        status.liveness = "pass".into();
    }

    (ready, status)
}

pub fn probe_status_json(status: &ProbeStatus) -> String {
    json!({
        "readiness": status.readiness,
        "liveness": status.liveness,
        "message": status.message,
    })
    .to_string()
}

async fn run_probe(
    probe: &ProbeConfig,
    desired: &DesiredWorkload,
    container_name: &str,
) -> Result<()> {
    let kind = probe.r#type.trim().to_ascii_lowercase();
    if kind.is_empty() || kind == "none" || kind == "off" {
        return Ok(());
    }
    let port = if probe.port > 0 {
        probe.port
    } else {
        desired.host_port
    };
    let timeout_secs = if probe.timeout_seconds > 0 {
        probe.timeout_seconds as u64
    } else {
        3
    };
    match kind.as_str() {
        "tcp" => {
            timeout(
                Duration::from_secs(timeout_secs),
                TcpStream::connect(("127.0.0.1", port as u16)),
            )
            .await
            .context("tcp timeout")?
            .context("tcp connect")?;
            Ok(())
        }
        "http" => {
            let path = if probe.path.trim().is_empty() {
                "/"
            } else {
                probe.path.trim()
            };
            let url = format!("http://127.0.0.1:{port}{path}");
            let client = reqwest::Client::builder()
                .timeout(Duration::from_secs(timeout_secs))
                .build()
                .context("http client")?;
            let resp = client.get(&url).send().await.context("http get")?;
            if resp.status().as_u16() >= 500 {
                bail!("http status {}", resp.status());
            }
            Ok(())
        }
        "exec" => {
            if probe.command.is_empty() {
                bail!("exec probe missing command");
            }
            let mut args = vec!["exec".to_string(), container_name.to_string()];
            args.extend(probe.command.iter().cloned());
            let output = timeout(
                Duration::from_secs(timeout_secs),
                Command::new("docker").args(&args).output(),
            )
            .await
            .context("exec timeout")?
            .context("docker exec")?;
            if !output.status.success() {
                bail!(
                    "exec exit {}: {}",
                    output.status.code().unwrap_or(-1),
                    String::from_utf8_lossy(&output.stderr).trim()
                );
            }
            Ok(())
        }
        other => bail!("unknown probe type {other}"),
    }
}
