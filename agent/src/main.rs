mod capabilities;
mod client;
mod config;
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
                match client
                    .heartbeat(&state, &caps, memory_free, 0.0, Vec::new())
                    .await
                {
                    Ok(desired) => {
                        if backoff > Duration::from_secs(1) {
                            info!("heartbeat restored");
                        }
                        backoff = Duration::from_secs(1);
                        if !desired.is_empty() {
                            if let Some(runtime) = runtime.as_ref() {
                                info!(count = desired.len(), "reconciling desired workloads");
                                let observed = runtime.reconcile(&desired).await;
                                if let Err(error) = client.report_observed(&state, &observed).await {
                                    warn!(%error, "report observed failed");
                                }
                            } else {
                                warn!(count = desired.len(), "desired workloads pending but no container runtime");
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
