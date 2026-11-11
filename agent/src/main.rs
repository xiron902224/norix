mod build;
mod capabilities;
mod client;
mod config;
mod edge;
mod network;
mod proto_num;
mod runtime;

use std::path::Path;
use std::time::Duration;

use anyhow::{bail, Context, Result};
use tokio::signal;
use tokio::time::{sleep, Instant};
use tracing::{info, warn};

use crate::capabilities::detect;
use crate::client::{AgentClient, NodeState};
use crate::config::Config;
use crate::runtime::Runtime;

#[tokio::main]
async fn main() -> Result<()> {
    let filter = std::env::var("NORIX_AGENT_LOG_LEVEL")
        .ok()
        .and_then(|value| tracing_subscriber::EnvFilter::try_new(value).ok())
        .unwrap_or_else(|| tracing_subscriber::EnvFilter::new("info"));
    tracing_subscriber::fmt().with_env_filter(filter).init();

    let config = Config::from_env()?;
    config.ensure_workdir()?;
    let client = AgentClient::new(config.api_base())?;
    let caps = detect(&config.agent_version);
    info!(
        cpu = caps.cpu_cores,
        memory_mb = caps.memory_total_mb,
        docker = caps.docker_socket,
        containerd = caps.containerd_socket,
        kvm_level = caps.kvm_level,
        "detected capabilities"
    );

    let runtime = match Runtime::detect().await {
        Ok(value) => Some(value),
        Err(error) => {
            warn!(%error, "container runtime unavailable; heartbeats only until docker/nerdctl is installed");
            None
        }
    };
    let state = load_or_enroll(&client, &config, &caps).await?;
    info!(node_id = %state.node_id, "agent enrolled");

    let mut backoff = Duration::from_secs(1);
    let max_backoff = Duration::from_secs(30);
    let heartbeat = Duration::from_secs(config.heartbeat_secs);

    loop {
        tokio::select! {
            _ = signal::ctrl_c() => {
                info!("shutdown signal received");
                break;
            }
            _ = sleep(heartbeat) => {
                let memory_free = memory_free_mb();
                let cpu_usage = cpu_usage_percent();
                match client
                    .heartbeat(&state, &caps, memory_free, cpu_usage, Vec::new())
                    .await
                {
                    Ok(desired) => {
                        if backoff > Duration::from_secs(1) {
                            info!("heartbeat restored");
                        }
                        backoff = Duration::from_secs(1);
                        if let Some(runtime) = runtime.as_ref() {
                            if !desired.desired_builds.is_empty() {
                                info!(count = desired.desired_builds.len(), "running desired builds");
                                for observed in runtime.run_builds(&desired.desired_builds).await {
                                    if let Err(error) = client.report_build(&state, &observed).await {
                                        warn!(%error, "report build failed");
                                    }
                                }
                            }
                            if !desired.desired_workloads.is_empty() {
                                info!(count = desired.desired_workloads.len(), "reconciling desired workloads");
                                let observed = runtime.reconcile(&desired.desired_workloads).await;
                                if let Err(error) = client.report_observed(&state, &observed).await {
                                    warn!(%error, "report observed failed");
                                }
                            }
                        } else if !desired.desired_workloads.is_empty() || !desired.desired_builds.is_empty() {
                            warn!("desired builds/workloads pending but no container runtime");
                        }
                        if let Some(plan) = desired.desired_edge.as_ref() {
                            info!(generation = plan.generation, "applying edge plan");
                            let observed = edge::apply_edge(plan).await;
                            if let Err(error) = client.report_edge(&state, &observed).await {
                                warn!(%error, "report edge failed");
                            }
                        }
                        if let Some(plan) = desired.desired_network.as_ref() {
                            info!(generation = plan.generation, "applying network plan");
                            let observed = network::apply_network(plan, &config.workdir).await;
                            if let Err(error) = client.report_network(&state, &observed).await {
                                warn!(%error, "report network failed");
                            }
                        }
                    }
                    Err(error) => {
                        warn!(%error, retry_in = ?backoff, "heartbeat failed; reconnecting with backoff");
                        let _started = Instant::now();
                        sleep(backoff).await;
                        backoff = (backoff * 2).min(max_backoff);
                    }
                }
            }
        }
    }
    Ok(())
}

async fn load_or_enroll(
    client: &AgentClient,
    config: &Config,
    caps: &capabilities::Capabilities,
) -> Result<NodeState> {
    if let Some(state) = read_state(&config.state_path())? {
        info!(node_id = %state.node_id, "loaded saved node credentials");
        return Ok(state);
    }
    let Some(join_token) = config.join_token.as_deref() else {
        bail!(
            "missing NORIX_JOIN_TOKEN and no saved state at {}",
            config.state_path().display()
        );
    };
    let state = client
        .enroll(join_token, &config.node_name, caps)
        .await
        .context("enroll node")?;
    write_state(&config.state_path(), &state)?;
    Ok(state)
}

fn read_state(path: &Path) -> Result<Option<NodeState>> {
    if !path.exists() {
        return Ok(None);
    }
    let raw = std::fs::read_to_string(path)
        .with_context(|| format!("read state {}", path.display()))?;
    let state = serde_json::from_str(&raw)
        .with_context(|| format!("parse state {}", path.display()))?;
    Ok(Some(state))
}

fn write_state(path: &Path, state: &NodeState) -> Result<()> {
    let raw = serde_json::to_string_pretty(state).context("encode state")?;
    std::fs::write(path, raw).with_context(|| format!("write state {}", path.display()))?;
    Ok(())
}

fn memory_free_mb() -> i64 {
    let Ok(contents) = std::fs::read_to_string("/proc/meminfo") else {
        return 0;
    };
    for line in contents.lines() {
        if let Some(rest) = line.strip_prefix("MemAvailable:") {
            let kb: i64 = rest
                .split_whitespace()
                .next()
                .and_then(|value| value.parse().ok())
                .unwrap_or(0);
            return kb / 1024;
        }
    }
    0
}

fn cpu_usage_percent() -> f64 {
    let sample = |path: &str| -> Option<(u64, u64)> {
        let contents = std::fs::read_to_string(path).ok()?;
        let line = contents.lines().next()?;
        let mut parts = line.split_whitespace();
        if parts.next()? != "cpu" {
            return None;
        }
        let mut values = [0u64; 8];
        for slot in values.iter_mut() {
            *slot = parts.next()?.parse().ok()?;
        }
        let idle = values[3] + values[4];
        let total: u64 = values.iter().sum();
        Some((idle, total))
    };
    let Some((idle1, total1)) = sample("/proc/stat") else {
        return 0.0;
    };
    std::thread::sleep(Duration::from_millis(120));
    let Some((idle2, total2)) = sample("/proc/stat") else {
        return 0.0;
    };
    let idle_delta = idle2.saturating_sub(idle1) as f64;
    let total_delta = total2.saturating_sub(total1) as f64;
    if total_delta <= 0.0 {
        return 0.0;
    }
    ((1.0 - idle_delta / total_delta) * 100.0).clamp(0.0, 100.0)
}
