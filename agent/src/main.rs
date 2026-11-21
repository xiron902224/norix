mod build;
mod capabilities;
mod client;
mod compose;
mod compose_model;
mod config;
mod edge;
mod gitssrf;
mod managed_db;
mod microvm;
mod network;
mod probes;
mod proto_num;
mod runtime;
mod update;
mod wasm;

use std::path::Path;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{bail, Context, Result};
use std::future::Future;
use tokio::signal;
use tokio::sync::{mpsc, Mutex};
use tokio::time::{sleep, Instant};
use tracing::{info, warn};

use crate::capabilities::detect;
use crate::client::{
    AgentClient, DesiredBuild, HeartbeatResult, NodeState, ObservedBuild, WatchDesiredResult,
};
use crate::config::Config;
use crate::runtime::{is_container_runtime, is_microvm_runtime, Runtime};

/// Runs one build. While it waits for a slot it reports `queued`, so the control plane
/// can show the queue. When it finishes it reports the result (#24).
async fn run_build_task(
    runtime: &Runtime,
    client: &AgentClient,
    state: &NodeState,
    build: &DesiredBuild,
) {
    let slots = build::slots();
    let permit = match slots.try_acquire() {
        Some(permit) => permit,
        None => {
            let waiting = ObservedBuild {
                id: build.id.clone(),
                status: "queued".into(),
                image: String::new(),
                message: "waiting for a build slot".into(),
                log_lines: vec![format!(
                    "queued: {} of {} build slots busy",
                    slots.busy(),
                    slots.limit()
                )],
            };
            report_build_result(client, state, &waiting).await;
            slots.acquire().await
        }
    };
    let observed = runtime.run_build_with_timeout(build).await;
    drop(permit);
    report_build_result(client, state, &observed).await;
}

async fn report_build_result(client: &AgentClient, state: &NodeState, observed: &ObservedBuild) {
    if let Err(error) = retry_operation(
        "report_build",
        || client.report_build(state, observed),
        3,
        Duration::from_millis(250),
    )
    .await
    {
        warn!(%error, build_id = %observed.id, "report build failed after retries");
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("update") {
        let version = args
            .iter()
            .position(|a| a == "--version")
            .and_then(|i| args.get(i + 1))
            .cloned()
            .or_else(|| args.get(1).filter(|a| !a.starts_with('-')).cloned());
        return update::run_cli_update(version).await;
    }
    if args.first().map(String::as_str) == Some("uninstall") {
        let purge = args.iter().any(|a| a == "--purge");
        return update::run_cli_uninstall(purge);
    }
    let check_only = args.iter().any(|arg| arg == "--check" || arg == "check");

    let filter = std::env::var("NORIX_AGENT_LOG_LEVEL")
        .ok()
        .and_then(|value| tracing_subscriber::EnvFilter::try_new(value).ok())
        .unwrap_or_else(|| tracing_subscriber::EnvFilter::new("info"));
    tracing_subscriber::fmt().with_env_filter(filter).init();

    let config = Config::from_env()?;

    if check_only {
        // check does not require join token / node-state — only config + optional probes.
        if config.control_plane_url.trim().is_empty() {
            bail!("NORIX_CONTROL_PLANE_URL cannot be empty");
        }
        let _ = config.ensure_workdir();
        let enrolled = config.state_path().exists() || config.join_token.is_some();
        let health_url = format!("{}/healthz", config.control_plane_url);
        let cp_reachable = match reqwest::Client::builder()
            .timeout(Duration::from_secs(3))
            .build()
        {
            Ok(client) => match client.get(&health_url).send().await {
                Ok(response) if response.status().is_success() => true,
                Ok(response) => {
                    warn!(status = %response.status(), url = %health_url, "control plane healthz not ok");
                    false
                }
                Err(error) => {
                    warn!(%error, url = %health_url, "control plane unreachable");
                    false
                }
            },
            Err(error) => {
                warn!(%error, "could not build http client for healthz probe");
                false
            }
        };
        match Runtime::detect().await {
            Ok(_) => info!("container runtime detected"),
            Err(error) => {
                warn!(
                    %error,
                    "container runtime unavailable (heartbeats-only until docker/nerdctl); see docs/user/nodes-agents.md"
                )
            }
        }
        info!(
            version = %config.agent_version,
            control_plane = %config.control_plane_url,
            workdir = %config.workdir.display(),
            enrolled,
            cp_reachable,
            "agent check passed"
        );
        return Ok(());
    }

    config.ensure_workdir()?;
    config.validate_for_startup()?;

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
            warn!(
                %error,
                "container runtime unavailable; heartbeats only until docker/nerdctl is installed (docs/user/nodes-agents.md)"
            );
            None
        }
    };
    let compose_stacks = runtime
        .as_ref()
        .map(|rt| compose::ComposeStacks::new(rt.bin(), &config.workdir));
    let state = Arc::new(Mutex::new(load_or_enroll(&client, &config, &caps).await?));
    {
        let guard = state.lock().await;
        info!(node_id = %guard.node_id, "agent enrolled");
    }

    let mut backoff = Duration::from_secs(1);
    let max_backoff = Duration::from_secs(30);
    let heartbeat = Duration::from_secs(config.heartbeat_secs);
    let mut last_observed_dbs: Vec<client::ObservedManagedDatabase> = Vec::new();
    let apply_lock = Arc::new(Mutex::new(()));
    let last_watch_fp = Arc::new(Mutex::new(String::new()));

    // WatchDesired long-poll (#44): parallel with heartbeats; wakes on desired change.
    let (watch_tx, mut watch_rx) = mpsc::channel::<WatchDesiredResult>(4);
    if config.watch_desired {
        let client_w = client.clone();
        let state_w = Arc::clone(&state);
        let timeout = config.watch_timeout_secs;
        let fp_w = Arc::clone(&last_watch_fp);
        tokio::spawn(async move {
            let mut backoff = Duration::from_secs(1);
            let max_backoff = Duration::from_secs(30);
            loop {
                let (node_snapshot, known) = {
                    let s = state_w.lock().await;
                    let fp = fp_w.lock().await.clone();
                    (s.clone(), fp)
                };
                match client_w
                    .watch_desired(&node_snapshot, &known, timeout)
                    .await
                {
                    Ok(watch) => {
                        if backoff > Duration::from_secs(1) {
                            info!("watch desired restored");
                        }
                        backoff = Duration::from_secs(1);
                        {
                            let mut fp = fp_w.lock().await;
                            if !watch.fingerprint.is_empty() {
                                *fp = watch.fingerprint.clone();
                            }
                        }
                        if watch.changed {
                            info!(
                                fingerprint = %watch.fingerprint,
                                timed_out = watch.timed_out,
                                "watch desired changed; reconciling"
                            );
                            if watch_tx.send(watch).await.is_err() {
                                return;
                            }
                        }
                    }
                    Err(error) => {
                        warn!(%error, retry_in = ?backoff, "watch desired failed; reconnecting with backoff");
                        sleep(backoff).await;
                        backoff = next_backoff(backoff, max_backoff);
                    }
                }
            }
        });
        info!(
            timeout_secs = config.watch_timeout_secs,
            "WatchDesired long-poll enabled"
        );
    } else {
        info!("WatchDesired disabled (NORIX_WATCH_DESIRED=0); heartbeat-only reconcile");
    }

    loop {
        tokio::select! {
            _ = signal::ctrl_c() => {
                info!("shutdown signal received");
                break;
            }
            Some(watch) = watch_rx.recv() => {
                let _guard = apply_lock.lock().await;
                let mut state_guard = state.lock().await;
                apply_desired(
                    &config,
                    &client,
                    &mut state_guard,
                    &caps,
                    runtime.as_ref(),
                    compose_stacks.as_ref(),
                    &watch.desired,
                    &mut last_observed_dbs,
                    "watch",
                )
                .await;
            }
            _ = sleep(heartbeat) => {
                let memory_free = memory_free_mb();
                let cpu_usage = cpu_usage_percent();
                let running_ids = if let Some(runtime) = runtime.as_ref() {
                    runtime.list_running_workload_ids().await
                } else {
                    Vec::new()
                };
                let state_snapshot = state.lock().await.clone();
                match client
                    .heartbeat(
                        &state_snapshot,
                        &caps,
                        memory_free,
                        cpu_usage,
                        running_ids,
                        &last_observed_dbs,
                    )
                    .await
                {
                    Ok(desired) => {
                        if backoff > Duration::from_secs(1) {
                            info!("heartbeat restored");
                        }
                        backoff = Duration::from_secs(1);
                        let _guard = apply_lock.lock().await;
                        let mut state_guard = state.lock().await;
                        apply_desired(
                            &config,
                            &client,
                            &mut state_guard,
                            &caps,
                            runtime.as_ref(),
                            compose_stacks.as_ref(),
                            &desired,
                            &mut last_observed_dbs,
                            "heartbeat",
                        )
                        .await;
                    }
                    Err(error) => {
                        warn!(%error, retry_in = ?backoff, "heartbeat failed; reconnecting with backoff");
                        let _started = Instant::now();
                        sleep(backoff).await;
                        backoff = next_backoff(backoff, max_backoff);
                    }
                }
            }
        }
    }
    Ok(())
}

/// Applies desired state from Heartbeat or WatchDesired. Serialized by the caller (#44).
#[allow(clippy::too_many_arguments)]
async fn apply_desired(
    config: &Config,
    client: &AgentClient,
    state: &mut NodeState,
    _caps: &capabilities::Capabilities,
    runtime: Option<&Runtime>,
    compose_stacks: Option<&compose::ComposeStacks>,
    desired: &HeartbeatResult,
    last_observed_dbs: &mut Vec<client::ObservedManagedDatabase>,
    source: &str,
) {
    if let Some(new_token) = desired.rotated_node_token.as_deref() {
        state.node_token = new_token.to_string();
        if let Err(error) = write_state(&config.state_path(), state) {
            warn!(%error, "failed to persist rotated node token");
        } else {
            info!("persisted rotated node token from control plane");
        }
    }
    if let Some(target) = desired
        .desired_agent_version
        .as_deref()
        .map(str::trim)
        .filter(|v| !v.is_empty())
    {
        let current = config.agent_version.trim().trim_start_matches('v');
        let want = target.trim_start_matches('v');
        if !want.is_empty() && want != current {
            info!(from = %current, to = %want, source, "control plane requested agent update");
            if let Err(error) = update::apply_desired_version(config, target).await {
                warn!(%error, "agent self-update failed");
            }
        }
    }
    if let Some(stacks) = compose_stacks {
        if !desired.desired_compose_stacks.is_empty() {
            stacks.reconcile(client, state, &desired.desired_compose_stacks);
        }
    }
    if let Some(runtime) = runtime {
        if !desired.desired_builds.is_empty() {
            // A git build can take many minutes. Running it inside the
            // heartbeat loop marked the node offline and froze every
            // other action (deletes, image deploys) until it finished.
            // Each build gets its own task. build::slots() limits how many run at once (#24).
            info!(
                count = desired.desired_builds.len(),
                limit = build::slots().limit(),
                source,
                "scheduling desired builds"
            );
            for build in desired.desired_builds.clone() {
                let runtime = runtime.clone();
                let client = client.clone();
                let state = state.clone();
                tokio::spawn(async move {
                    run_build_task(&runtime, &client, &state, &build).await;
                });
            }
        }
        if !desired.desired_workloads.is_empty() {
            info!(
                count = desired.desired_workloads.len(),
                source, "reconciling desired workloads"
            );
            let mut observed = Vec::new();
            let mut containers = Vec::new();
            for item in &desired.desired_workloads {
                if is_microvm_runtime(&item.runtime) {
                    observed.push(microvm::reconcile_one(&config.workdir, item).await);
                } else if is_container_runtime(&item.runtime) {
                    containers.push(item.clone());
                } else if runtime::is_wasm_runtime(&item.runtime) {
                    observed.push(client::ObservedWorkload {
                        id: item.id.clone(),
                        status: "failed".into(),
                        container_id: String::new(),
                        observed_generation: item.desired_generation,
                        message: "use Runtimes / DesiredWasmPlan for wasm functions".into(),
                        host_port: item.host_port,
                        log_lines: vec!["skipped wasm workload".into()],
                        ready: true,
                        probe_status_json: "{}".into(),
                    });
                } else {
                    observed.push(client::ObservedWorkload {
                        id: item.id.clone(),
                        status: "failed".into(),
                        container_id: String::new(),
                        observed_generation: item.desired_generation,
                        message: format!("unsupported runtime {}", item.runtime),
                        host_port: item.host_port,
                        log_lines: vec![],
                        ready: true,
                        probe_status_json: "{}".into(),
                    });
                }
            }
            if !containers.is_empty() {
                observed.extend(runtime.reconcile(&containers).await);
            }
            if let Err(error) = retry_operation(
                "report_observed",
                || client.report_observed(state, &observed),
                3,
                Duration::from_millis(250),
            )
            .await
            {
                warn!(%error, "report observed failed after retries");
            }
        }
        if !desired.desired_managed_databases.is_empty() {
            info!(
                count = desired.desired_managed_databases.len(),
                source, "reconciling managed databases"
            );
            *last_observed_dbs = runtime
                .reconcile_managed_databases(&desired.desired_managed_databases)
                .await;
        }
    } else if !desired.desired_workloads.is_empty()
        || !desired.desired_builds.is_empty()
        || !desired.desired_managed_databases.is_empty()
    {
        warn!(
            source,
            "desired builds/workloads/DBs pending but no container runtime; degraded mode active"
        );
    }
    if let Some(plan) = desired.desired_edge.as_ref() {
        info!(generation = plan.generation, source, "applying edge plan");
        let observed = edge::apply_edge(plan).await;
        if let Err(error) = retry_operation(
            "report_edge",
            || client.report_edge(state, &observed),
            3,
            Duration::from_millis(300),
        )
        .await
        {
            warn!(%error, "report edge failed after retries");
        }
    }
    if let Some(plan) = desired.desired_network.as_ref() {
        info!(
            generation = plan.generation,
            source, "applying network plan"
        );
        let observed = network::apply_network(plan, &config.workdir).await;
        if let Err(error) = retry_operation(
            "report_network",
            || client.report_network(state, &observed),
            3,
            Duration::from_millis(300),
        )
        .await
        {
            warn!(%error, "report network failed after retries");
        }
    }
    if let Some(plan) = desired.desired_wasm.as_ref() {
        info!(
            generation = plan.generation,
            count = plan.functions.len(),
            source,
            "applying wasm plan"
        );
        let observed = wasm::apply_wasm_plan(&config.workdir, plan).await;
        if let Err(error) = retry_operation(
            "report_wasm",
            || client.report_wasm(state, &observed),
            3,
            Duration::from_millis(300),
        )
        .await
        {
            warn!(%error, "report wasm failed after retries");
        }
    }
}

/// Exponential backoff capped at `max` (heartbeat reconnect and operation retries).
fn next_backoff(current: Duration, max: Duration) -> Duration {
    current.saturating_mul(2).min(max)
}

async fn retry_operation<F, Fut, T>(
    label: &str,
    mut op: F,
    max_attempts: usize,
    initial_backoff: Duration,
) -> Result<T>
where
    F: FnMut() -> Fut,
    Fut: Future<Output = Result<T>>,
{
    let mut backoff = initial_backoff;
    for attempt in 1..=max_attempts {
        match op().await {
            Ok(value) => return Ok(value),
            Err(error) if attempt < max_attempts => {
                warn!(%error, operation = label, attempt, backoff = ?backoff, "operation failed; retrying");
                sleep(backoff).await;
                backoff = next_backoff(backoff, Duration::from_secs(10));
            }
            Err(error) => return Err(error),
        }
    }
    bail!("operation {label} exhausted retries")
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
    let raw =
        std::fs::read_to_string(path).with_context(|| format!("read state {}", path.display()))?;
    let state =
        serde_json::from_str(&raw).with_context(|| format!("parse state {}", path.display()))?;
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

#[cfg(test)]
mod tests {
    use super::{next_backoff, retry_operation};
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::time::Duration;

    #[test]
    fn next_backoff_doubles_until_cap() {
        let max = Duration::from_secs(30);
        assert_eq!(
            next_backoff(Duration::from_secs(1), max),
            Duration::from_secs(2)
        );
        assert_eq!(
            next_backoff(Duration::from_secs(16), max),
            Duration::from_secs(30)
        );
        assert_eq!(
            next_backoff(Duration::from_secs(30), max),
            Duration::from_secs(30)
        );
    }

    #[tokio::test]
    async fn retry_operation_retries_until_success() {
        let attempts = std::sync::Arc::new(AtomicUsize::new(0));
        let attempts_clone = attempts.clone();

        let value = retry_operation(
            "test_op",
            move || {
                let count = attempts_clone.fetch_add(1, Ordering::SeqCst);
                let future = async move {
                    if count < 2 {
                        Err(anyhow::anyhow!("transient fail"))
                    } else {
                        Ok("ok")
                    }
                };
                future
            },
            3,
            Duration::from_millis(1),
        )
        .await;

        assert!(value.is_ok());
        assert_eq!(attempts.load(Ordering::SeqCst), 3);
    }

    #[tokio::test]
    async fn retry_operation_exhausts_and_reports_error() {
        let value: Result<&str, anyhow::Error> = retry_operation(
            "test_op",
            || async { Err(anyhow::anyhow!("persistent fail")) },
            2,
            Duration::from_millis(1),
        )
        .await;

        assert!(value.is_err());
    }
}
