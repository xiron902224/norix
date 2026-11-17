use std::process::Stdio;

use anyhow::{bail, Context, Result};
use tokio::process::Command;
use tracing::{info, warn};

use crate::client::{DesiredWorkload, ObservedWorkload};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Engine {
    Docker,
    Nerdctl,
}

pub struct Runtime {
    engine: Engine,
}

impl Runtime {
    pub async fn detect() -> Result<Self> {
        if command_ok("docker", &["info"]).await {
            info!("using docker runtime");
            return Ok(Self {
                engine: Engine::Docker,
            });
        }
        if command_ok("nerdctl", &["info"]).await {
            info!("using nerdctl/containerd runtime");
            return Ok(Self {
                engine: Engine::Nerdctl,
            });
        }
        bail!("neither docker nor nerdctl is available; install a container runtime")
    }

    pub async fn reconcile(&self, desired: &[DesiredWorkload]) -> Vec<ObservedWorkload> {
        let mut observed = Vec::with_capacity(desired.len());
        for item in desired {
            if is_wasm_runtime(&item.runtime) {
                observed.push(ObservedWorkload {
                    id: item.id.clone(),
                    status: "failed".into(),
                    container_id: String::new(),
                    observed_generation: item.desired_generation,
                    message: "runtime=wasm is reconciled via DesiredWasmPlan, not container engine"
                        .into(),
                    host_port: item.host_port,
                    log_lines: vec!["skipped wasm workload in container reconcile".into()],
                    ready: true,
                    probe_status_json: "{}".into(),
                });
                continue;
            }
            match self.reconcile_one(item).await {
                Ok(mut value) => {
                    if let Some(op_msg) = self.apply_volume_op(item).await {
                        if value.message.is_empty() {
                            value.message = op_msg;
                        } else {
                            value.message = format!("{}; {}", value.message, op_msg);
                        }
                    }
                    observed.push(value);
                }
                Err(error) => {
                    warn!(workload = %item.id, %error, "reconcile failed");
                    let mut message = error.to_string();
                    if let Some(op_msg) = self.apply_volume_op(item).await {
                        message = format!("{message}; {op_msg}");
                    }
                    observed.push(ObservedWorkload {
                        id: item.id.clone(),
                        status: "failed".into(),
                        container_id: String::new(),
                        observed_generation: item.desired_generation,
                        message,
                        host_port: item.host_port,
                        log_lines: vec![format!("reconcile error: {error}")],
                        ready: true,
                        probe_status_json: "{}".into(),
                    });
                }
            }
        }
        observed
    }

    /// List workload IDs from containers labeled norix.workload_id (for heartbeat inventory).
    pub async fn list_running_workload_ids(&self) -> Vec<String> {
        let output = Command::new(self.bin())
            .args([
                "ps",
                "--filter",
                "label=norix.workload_id",
                "--format",
                "{{.Label \"norix.workload_id\"}}",
            ])
            .output()
            .await;
        let Ok(output) = output else {
            return Vec::new();
        };
        if !output.status.success() {
            return Vec::new();
        }
        String::from_utf8_lossy(&output.stdout)
            .lines()
            .map(str::trim)
            .filter(|line| !line.is_empty())
            .map(str::to_string)
            .collect()
    }

    async fn reconcile_one(&self, desired: &DesiredWorkload) -> Result<ObservedWorkload> {
        if !is_container_runtime(&desired.runtime) && !is_microvm_runtime(&desired.runtime) {
            bail!(
                "unsupported runtime {:?} for container engine",
                desired.runtime
            );
        }
        let name = container_name(&desired.id);
        if desired.action == "stop" {
            let _ = self.remove_container(&name).await;
            return Ok(ObservedWorkload {
                id: desired.id.clone(),
                status: "stopped".into(),
                container_id: String::new(),
                observed_generation: desired.desired_generation,
                message: String::new(),
                host_port: desired.host_port,
                log_lines: vec![format!("stopped {}", name)],
                ready: true,
                probe_status_json: "{}".into(),
            });
        }

        let ensure_only = desired.action == "none" || desired.action.is_empty();

        if let Some(existing) = self.inspect_running(&name).await? {
            let generation = existing
                .get("Config")
                .and_then(|cfg| cfg.get("Labels"))
                .and_then(|labels| labels.get("norix.generation"))
                .and_then(|value| value.as_str())
                .and_then(|value| value.parse::<i64>().ok())
                .unwrap_or(0);
            let image = existing
                .get("Config")
                .and_then(|cfg| cfg.get("Image"))
                .and_then(|value| value.as_str())
                .unwrap_or_default();
            if generation == desired.desired_generation && image_matches(image, &desired.image) {
                let logs = self.logs(&name, 40).await.unwrap_or_default();
                let container_id = existing
                    .get("Id")
                    .and_then(|value| value.as_str())
                    .unwrap_or_default()
                    .to_string();
                // Clean leftover surge container after a completed roll.
                let _ = self.remove_container(&format!("{name}-next")).await;
                let (ready, probe_status) = crate::probes::evaluate_probes(desired, &name).await;
                let mut observed = ObservedWorkload {
                    id: desired.id.clone(),
                    status: "running".into(),
                    container_id,
                    observed_generation: desired.desired_generation,
                    message: probe_status.message.clone(),
                    host_port: desired.host_port,
                    log_lines: logs,
                    ready,
                    probe_status_json: crate::probes::probe_status_json(&probe_status),
                };
                if probe_status.liveness == "fail" {
                    // Liveness failure: recreate container (#17).
                    let _ = self.remove_container(&name).await;
                    self.pull(&desired.image).await?;
                    let container_id = self.run(desired, &name).await?;
                    observed.container_id = container_id;
                    observed.status = "deploying".into();
                    observed.ready = false;
                    observed.message =
                        format!("liveness failed; restarted: {}", probe_status.message);
                }
                return Ok(observed);
            }
            if is_rolling(desired) {
                return self.reconcile_rolling(desired, &name).await;
            }
            let _ = self.remove_container(&name).await;
        } else if let Some(info) = self.inspect_any(&name).await? {
            // Present but unhealthy: on ensure-only ticks, surface failure
            // instead of rm/recreate thrashing that keeps status="running".
            if ensure_only {
                let logs = self.logs(&name, 40).await.unwrap_or_default();
                let container_id = info
                    .get("Id")
                    .and_then(|value| value.as_str())
                    .unwrap_or_default()
                    .to_string();
                let message = unhealthy_message(&info);
                let _ = self.remove_container(&name).await;
                return Ok(ObservedWorkload {
                    id: desired.id.clone(),
                    status: "failed".into(),
                    container_id,
                    observed_generation: desired.desired_generation,
                    message,
                    host_port: desired.host_port,
                    log_lines: logs,
                    ready: true,
                    probe_status_json: "{}".into(),
                });
            }
            let _ = self.remove_container(&name).await;
        } else if !ensure_only {
            let _ = self.remove_container(&name).await;
        }

        self.pull(&desired.image).await?;
        let container_id = self.run(desired, &name).await?;
        tokio::time::sleep(std::time::Duration::from_secs(2)).await;
        if self.inspect_running(&name).await?.is_some() {
            let logs = self.logs(&name, 40).await.unwrap_or_default();
            return Ok(ObservedWorkload {
                id: desired.id.clone(),
                status: "running".into(),
                container_id,
                observed_generation: desired.desired_generation,
                message: String::new(),
                host_port: desired.host_port,
                log_lines: logs,
                ready: true,
                probe_status_json: "{}".into(),
            });
        }
        let logs = self.logs(&name, 40).await.unwrap_or_default();
        let message = if let Some(info) = self.inspect_any(&name).await? {
            unhealthy_message(&info)
        } else {
            "container exited or is restarting after start".into()
        };
        Ok(ObservedWorkload {
            id: desired.id.clone(),
            status: "failed".into(),
            container_id,
            observed_generation: desired.desired_generation,
            message,
            host_port: desired.host_port,
            log_lines: logs,
            ready: true,
            probe_status_json: "{}".into(),
        })
    }

    /// Start-before-stop: bring up `{name}-next` on surge port, health-check, swap, drop old.
    async fn reconcile_rolling(
        &self,
        desired: &DesiredWorkload,
        name: &str,
    ) -> Result<ObservedWorkload> {
        let next = format!("{name}-next");
        let surge = if desired.surge_host_port > 0 {
            desired.surge_host_port
        } else {
            desired.host_port.saturating_add(1000).max(19000)
        };
        let mut surge_desired = desired.clone();
        surge_desired.host_port = surge;

        if let Some(existing_next) = self.inspect_running(&next).await? {
            let generation = existing_next
                .get("Config")
                .and_then(|cfg| cfg.get("Labels"))
                .and_then(|labels| labels.get("norix.generation"))
                .and_then(|value| value.as_str())
                .and_then(|value| value.parse::<i64>().ok())
                .unwrap_or(0);
            if generation != desired.desired_generation {
                let _ = self.remove_container(&next).await;
            }
        }

        if self.inspect_running(&next).await?.is_none() {
            self.pull(&desired.image).await?;
            let _ = self.remove_container(&next).await;
            let _ = self.run(&surge_desired, &next).await?;
            tokio::time::sleep(std::time::Duration::from_secs(2)).await;
        }

        if self.inspect_running(&next).await?.is_none() {
            let logs = self.logs(&next, 40).await.unwrap_or_default();
            let message = if let Some(info) = self.inspect_any(&next).await? {
                unhealthy_message(&info)
            } else {
                "rolling surge container exited".into()
            };
            let _ = self.remove_container(&next).await;
            return Ok(ObservedWorkload {
                id: desired.id.clone(),
                status: "failed".into(),
                container_id: String::new(),
                observed_generation: desired.desired_generation,
                message,
                host_port: desired.host_port,
                log_lines: logs,
                ready: true,
                probe_status_json: "{}".into(),
            });
        }

        if let Err(error) =
            readiness_check(surge, desired.container_port, &desired.readiness_path).await
        {
            let logs = self.logs(&next, 40).await.unwrap_or_default();
            return Ok(ObservedWorkload {
                id: desired.id.clone(),
                status: "deploying".into(),
                container_id: String::new(),
                observed_generation: desired.desired_generation.saturating_sub(1),
                message: format!("rolling readiness pending: {error}"),
                host_port: desired.host_port,
                log_lines: logs,
                ready: true,
                probe_status_json: "{}".into(),
            });
        }

        // Ready: cut over host_port to surge, stop old, rename next → canonical.
        let _ = self.remove_container(name).await;
        let _ = Command::new(self.bin())
            .args(["rename", &next, name])
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status()
            .await;
        let logs = self.logs(name, 40).await.unwrap_or_default();
        let container_id = self
            .inspect_running(name)
            .await?
            .and_then(|info| {
                info.get("Id")
                    .and_then(|value| value.as_str())
                    .map(str::to_string)
            })
            .unwrap_or_default();
        Ok(ObservedWorkload {
            id: desired.id.clone(),
            status: "running".into(),
            container_id,
            observed_generation: desired.desired_generation,
            message: format!("rolled via surge port {surge}"),
            host_port: surge,
            log_lines: logs,
            ready: true,
            probe_status_json: "{}".into(),
        })
    }

    fn bin(&self) -> &'static str {
        match self.engine {
            Engine::Docker => "docker",
            Engine::Nerdctl => "nerdctl",
        }
    }

    pub(crate) async fn engine_capture(
        &self,
        args: &[String],
        dir: Option<&std::path::Path>,
    ) -> Result<(bool, String)> {
        let mut command = Command::new(self.bin());
        command
            .args(args)
            .stdout(Stdio::piped())
            .stderr(Stdio::piped());
        if let Some(path) = dir {
            command.current_dir(path);
        }
        let output = command.output().await.context("engine command")?;
        let mut text = String::from_utf8_lossy(&output.stdout).to_string();
        text.push_str(&String::from_utf8_lossy(&output.stderr));
        Ok((output.status.success(), text))
    }

    async fn image_exists(&self, image: &str) -> bool {
        Command::new(self.bin())
            .args(["image", "inspect", image])
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status()
            .await
            .map(|status| status.success())
            .unwrap_or(false)
    }

    async fn pull(&self, image: &str) -> Result<()> {
        // Local builds tag images on the node; don't require a reachable registry.
        if self.image_exists(image).await {
            return Ok(());
        }
        let status = Command::new(self.bin())
            .args(["pull", image])
            .stdout(Stdio::null())
            .stderr(Stdio::piped())
            .status()
            .await
            .with_context(|| format!("pull {image}"))?;
        if !status.success() {
            bail!("pull {image} failed");
        }
        Ok(())
    }

    async fn run(&self, desired: &DesiredWorkload, name: &str) -> Result<String> {
        let port = format!("{}:{}", desired.host_port, desired.container_port.max(1));
        let memory = format!("{}m", desired.memory_limit_mb.max(64));
        let mut args = vec![
            "run".into(),
            "-d".into(),
            "--name".into(),
            name.into(),
            "--restart".into(),
            "unless-stopped".into(),
            "-p".into(),
            port,
            "--memory".into(),
            memory,
            "--label".into(),
            format!("norix.workload_id={}", desired.id),
            "--label".into(),
            format!("norix.generation={}", desired.desired_generation),
            "--label".into(),
            format!("norix.project_id={}", desired.project_id),
        ];
        if desired.cpu_shares > 0 {
            args.push("--cpu-shares".into());
            args.push(desired.cpu_shares.to_string());
        }
        if !desired.network_name.trim().is_empty() {
            let _ = self.ensure_network(desired.network_name.trim()).await;
            args.push("--network".into());
            args.push(desired.network_name.trim().into());
            if !desired.network_alias.trim().is_empty() {
                args.push("--network-alias".into());
                args.push(desired.network_alias.trim().into());
            }
        }
        for (key, value) in &desired.env_vars {
            args.push("-e".into());
            args.push(format!("{key}={value}"));
        }
        let workload_id = if desired.workload_id.trim().is_empty() {
            desired.id.as_str()
        } else {
            desired.workload_id.trim()
        };
        for mount in parse_volume_mounts(&desired.volume_mounts_json) {
            let docker_name = docker_volume_name(workload_id, &mount.name);
            let _ = Command::new(self.bin())
                .args(["volume", "create", &docker_name])
                .stdout(Stdio::null())
                .stderr(Stdio::null())
                .status()
                .await;
            let spec = if mount.read_only {
                format!("{docker_name}:{}:ro", mount.mount_path)
            } else {
                format!("{docker_name}:{}", mount.mount_path)
            };
            args.push("-v".into());
            args.push(spec);
        }
        args.push(desired.image.clone());

        let output = Command::new(self.bin())
            .args(&args)
            .output()
            .await
            .context("docker/nerdctl run")?;
        if !output.status.success() {
            bail!(
                "run failed: {}",
                String::from_utf8_lossy(&output.stderr).trim()
            );
        }
        Ok(String::from_utf8_lossy(&output.stdout).trim().to_string())
    }

    async fn remove_container(&self, name: &str) -> Result<()> {
        let _ = Command::new(self.bin())
            .args(["rm", "-f", name])
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status()
            .await;
        Ok(())
    }

    /// Runs pending backup/delete; returns norix-volume-op:… message for the control plane.
    async fn apply_volume_op(&self, desired: &DesiredWorkload) -> Option<String> {
        let raw = desired.volume_pending_op_json.trim();
        if raw.is_empty() || raw == "null" {
            return None;
        }
        let op: serde_json::Value = serde_json::from_str(raw).ok()?;
        let id = op.get("id")?.as_str()?.trim();
        let kind = op.get("op")?.as_str()?.trim().to_ascii_lowercase();
        let name = op.get("name")?.as_str()?.trim();
        if id.is_empty() || name.is_empty() {
            return None;
        }
        let workload_id = if desired.workload_id.trim().is_empty() {
            desired.id.as_str()
        } else {
            desired.workload_id.trim()
        };
        let docker_name = docker_volume_name(workload_id, name);
        match kind.as_str() {
            "backup" => match self.backup_named_volume(&docker_name, id).await {
                Ok(path) => Some(format!("norix-volume-op:{id}:done:{path}")),
                Err(err) => Some(format!("norix-volume-op:{id}:failed:{err}")),
            },
            "delete" => {
                // Best-effort stop container using the volume, then rm volume.
                let cname = container_name(&desired.id);
                let _ = self.remove_container(&cname).await;
                let output = Command::new(self.bin())
                    .args(["volume", "rm", "-f", &docker_name])
                    .output()
                    .await;
                match output {
                    Ok(out) if out.status.success() => {
                        Some(format!("norix-volume-op:{id}:done:"))
                    }
                    Ok(out) => Some(format!(
                        "norix-volume-op:{id}:failed:{}",
                        String::from_utf8_lossy(&out.stderr).trim()
                    )),
                    Err(err) => Some(format!("norix-volume-op:{id}:failed:{err}")),
                }
            }
            _ => None,
        }
    }

    async fn backup_named_volume(&self, docker_name: &str, op_id: &str) -> Result<String> {
        let dir = std::path::PathBuf::from("/var/lib/norix/volume-backups");
        tokio::fs::create_dir_all(&dir)
            .await
            .context("create volume backup dir")?;
        let host_path = dir.join(format!("{docker_name}-{op_id}.tar.gz"));
        let host_path_str = host_path.to_string_lossy().to_string();
        let mount_vol = format!("{docker_name}:/source:ro");
        let mount_out = format!("{}:/backup", dir.display());
        let archive = format!("/backup/{docker_name}-{op_id}.tar.gz");
        let output = Command::new(self.bin())
            .args([
                "run",
                "--rm",
                "-v",
                &mount_vol,
                "-v",
                &mount_out,
                "alpine:3.20",
                "tar",
                "czf",
                &archive,
                "-C",
                "/source",
                ".",
            ])
            .output()
            .await
            .context("volume backup docker run")?;
        if !output.status.success() {
            bail!(
                "backup failed: {}",
                String::from_utf8_lossy(&output.stderr).trim()
            );
        }
        Ok(host_path_str)
    }

    async fn ensure_network(&self, name: &str) -> Result<()> {
        let inspect = Command::new(self.bin())
            .args(["network", "inspect", name])
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status()
            .await;
        if inspect.map(|s| s.success()).unwrap_or(false) {
            return Ok(());
        }
        let status = Command::new(self.bin())
            .args(["network", "create", name])
            .stdout(Stdio::null())
            .stderr(Stdio::piped())
            .status()
            .await
            .context("docker network create")?;
        if !status.success() {
            // Race: another reconciler created it.
            let again = Command::new(self.bin())
                .args(["network", "inspect", name])
                .stdout(Stdio::null())
                .stderr(Stdio::null())
                .status()
                .await;
            if again.map(|s| s.success()).unwrap_or(false) {
                return Ok(());
            }
            bail!("could not create network {name}");
        }
        Ok(())
    }

    async fn inspect_any(&self, name: &str) -> Result<Option<serde_json::Value>> {
        let output = Command::new(self.bin())
            .args(["inspect", name])
            .output()
            .await
            .context("inspect container")?;
        if !output.status.success() {
            return Ok(None);
        }
        let parsed: serde_json::Value =
            serde_json::from_slice(&output.stdout).context("parse inspect json")?;
        let Some(first) = parsed.as_array().and_then(|items| items.first()) else {
            return Ok(None);
        };
        Ok(Some(first.clone()))
    }

    async fn inspect_running(&self, name: &str) -> Result<Option<serde_json::Value>> {
        let Some(first) = self.inspect_any(name).await? else {
            return Ok(None);
        };
        let running = first
            .pointer("/State/Running")
            .and_then(|value| value.as_bool())
            .unwrap_or(false);
        let restarting = first
            .pointer("/State/Restarting")
            .and_then(|value| value.as_bool())
            .unwrap_or(false);
        let oom_killed = first
            .pointer("/State/OOMKilled")
            .and_then(|value| value.as_bool())
            .unwrap_or(false);
        if running && !restarting && !oom_killed {
            Ok(Some(first))
        } else {
            Ok(None)
        }
    }

    async fn logs(&self, name: &str, tail: usize) -> Result<Vec<String>> {
        let output = Command::new(self.bin())
            .args(["logs", "--tail", &tail.to_string(), name])
            .output()
            .await
            .context("container logs")?;
        let combined = format!(
            "{}\n{}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
        Ok(combined
            .lines()
            .map(str::trim)
            .filter(|line| !line.is_empty())
            .map(str::to_string)
            .collect())
    }
}

#[derive(Debug, Clone)]
struct VolumeMountSpec {
    name: String,
    mount_path: String,
    read_only: bool,
}

fn parse_volume_mounts(raw: &str) -> Vec<VolumeMountSpec> {
    let raw = raw.trim();
    if raw.is_empty() || raw == "null" || raw == "[]" {
        return Vec::new();
    }
    let Ok(value) = serde_json::from_str::<serde_json::Value>(raw) else {
        return Vec::new();
    };
    let Some(arr) = value.as_array() else {
        return Vec::new();
    };
    let mut out = Vec::new();
    for item in arr {
        let name = item
            .get("name")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .trim()
            .to_string();
        let mount_path = item
            .get("mount_path")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .trim()
            .to_string();
        if name.is_empty() || !mount_path.starts_with('/') || mount_path == "/" {
            continue;
        }
        if mount_path.contains("..") {
            continue;
        }
        let read_only = item
            .get("read_only")
            .and_then(|v| v.as_bool())
            .unwrap_or(false);
        out.push(VolumeMountSpec {
            name,
            mount_path,
            read_only,
        });
    }
    out
}

fn docker_volume_name(workload_id: &str, logical: &str) -> String {
    let id = workload_id.replace('-', "");
    let id = if id.len() > 12 { &id[..12] } else { &id };
    let mut name = logical.to_ascii_lowercase();
    name = name
        .chars()
        .map(|c| match c {
            'a'..='z' | '0'..='9' | '-' | '_' | '.' => c,
            _ => '-',
        })
        .collect();
    if name.is_empty() {
        name = "data".into();
    }
    if name.len() > 40 {
        name.truncate(40);
    }
    format!("norix-wv-{id}-{name}")
}

fn container_name(workload_id: &str) -> String {
    let compact: String = workload_id
        .chars()
        .filter(|ch| ch.is_ascii_hexdigit())
        .collect();
    format!("norix-{}", &compact[..compact.len().min(12)])
}

fn is_rolling(desired: &DesiredWorkload) -> bool {
    let strategy = desired.update_strategy.trim().to_ascii_lowercase();
    strategy.is_empty() || strategy == "rolling"
}

async fn readiness_check(host_port: i32, _container_port: i32, path: &str) -> Result<()> {
    let path = if path.trim().is_empty() {
        "/"
    } else if path.starts_with('/') {
        path.trim()
    } else {
        return tcp_ready(host_port).await;
    };
    let url = format!("http://127.0.0.1:{host_port}{path}");
    let client = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(3))
        .build()
        .context("readiness http client")?;
    match client.get(&url).send().await {
        Ok(resp) if resp.status().as_u16() < 500 => Ok(()),
        Ok(resp) => bail!("readiness HTTP {}", resp.status()),
        Err(_) => tcp_ready(host_port).await,
    }
}

async fn tcp_ready(host_port: i32) -> Result<()> {
    use tokio::net::TcpStream;
    match TcpStream::connect(("127.0.0.1", host_port as u16)).await {
        Ok(_) => Ok(()),
        Err(err) => bail!("readiness TCP {host_port}: {err}"),
    }
}

pub fn is_container_runtime(runtime: &str) -> bool {
    matches!(
        runtime.trim().to_ascii_lowercase().as_str(),
        "" | "container"
            | "docker-compose"
            | "static-site"
            | "runtime_type_container"
            | "1"
            | "2"
            | "5"
    )
}

pub fn is_microvm_runtime(runtime: &str) -> bool {
    matches!(
        runtime.trim().to_ascii_lowercase().as_str(),
        "microvm" | "runtime_type_microvm" | "3"
    )
}

pub fn is_wasm_runtime(runtime: &str) -> bool {
    matches!(
        runtime.trim().to_ascii_lowercase().as_str(),
        "wasm" | "runtime_type_wasm" | "4"
    )
}

fn normalize_image(value: &str) -> &str {
    value
        .trim()
        .trim_matches('"')
        .trim_start_matches("docker.io/")
        .trim_start_matches("index.docker.io/")
        .trim_start_matches("library/")
}

fn unhealthy_message(info: &serde_json::Value) -> String {
    let status = info
        .pointer("/State/Status")
        .and_then(|value| value.as_str())
        .unwrap_or_default();
    let error = info
        .pointer("/State/Error")
        .and_then(|value| value.as_str())
        .unwrap_or_default();
    let exit = info
        .pointer("/State/ExitCode")
        .and_then(|value| value.as_i64())
        .unwrap_or(0);
    let restarting = info
        .pointer("/State/Restarting")
        .and_then(|value| value.as_bool())
        .unwrap_or(false);
    let mut parts = Vec::new();
    if !status.is_empty() {
        parts.push(format!("status={status}"));
    }
    if restarting {
        parts.push("restarting".into());
    }
    if exit != 0 {
        parts.push(format!("exit={exit}"));
    }
    if !error.is_empty() {
        parts.push(error.to_string());
    }
    if parts.is_empty() {
        "container unhealthy".into()
    } else {
        format!("container unhealthy: {}", parts.join(" "))
    }
}

fn image_matches(running: &str, desired: &str) -> bool {
    let running = normalize_image(running);
    let desired = normalize_image(desired);

    if running == desired {
        return true;
    }

    let running_last = running.rsplit('/').next().unwrap_or(running);
    let desired_last = desired.rsplit('/').next().unwrap_or(desired);
    if running_last == desired_last {
        return true;
    }

    running.ends_with(&format!("/{desired}"))
}

async fn command_ok(bin: &str, args: &[&str]) -> bool {
    Command::new(bin)
        .args(args)
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .await
        .map(|status| status.success())
        .unwrap_or(false)
}

#[cfg(test)]
mod tests {
    use super::image_matches;

    #[test]
    fn matches_same_image_and_registry_prefix() {
        assert!(image_matches(
            "docker.io/library/nginx:latest",
            "nginx:latest"
        ));
        assert!(image_matches("ghcr.io/acme/api:1.2.3", "api:1.2.3"));
        assert!(image_matches("ghcr.io/acme/nginx", "nginx"));
    }

    #[test]
    fn does_not_match_other_images_by_substring() {
        assert!(!image_matches(
            "ghcr.io/acme/nginx-prod:latest",
            "nginx:latest"
        ));
        assert!(!image_matches(
            "ghcr.io/acme/not-nginx:latest",
            "nginx:latest"
        ));
        assert!(!image_matches("ghcr.io/acme/myapi:1.2.3", "api:1.2.3"));
        assert!(!image_matches(
            "ghcr.io/acme/my-nginx:latest",
            "nginx:latest"
        ));
    }
}
