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
            match self.reconcile_one(item).await {
                Ok(value) => observed.push(value),
                Err(error) => {
                    warn!(workload = %item.id, %error, "reconcile failed");
                    observed.push(ObservedWorkload {
                        id: item.id.clone(),
                        status: "failed".into(),
                        container_id: String::new(),
                        observed_generation: item.desired_generation,
                        message: error.to_string(),
                        host_port: item.host_port,
                        log_lines: vec![format!("reconcile error: {error}")],
                    });
                }
            }
        }
        observed
    }

    async fn reconcile_one(&self, desired: &DesiredWorkload) -> Result<ObservedWorkload> {
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
            });
        }

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
                return Ok(ObservedWorkload {
                    id: desired.id.clone(),
                    status: "running".into(),
                    container_id,
                    observed_generation: desired.desired_generation,
                    message: String::new(),
                    host_port: desired.host_port,
                    log_lines: logs,
                });
            }
            let _ = self.remove_container(&name).await;
        }

        self.pull(&desired.image).await?;
        let container_id = self.run(desired, &name).await?;
        let logs = self.logs(&name, 40).await.unwrap_or_default();
        Ok(ObservedWorkload {
            id: desired.id.clone(),
            status: "running".into(),
            container_id,
            observed_generation: desired.desired_generation,
            message: String::new(),
            host_port: desired.host_port,
            log_lines: logs,
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
        command.args(args).stdout(Stdio::piped()).stderr(Stdio::piped());
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
        for (key, value) in &desired.env_vars {
            args.push("-e".into());
            args.push(format!("{key}={value}"));
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

    async fn inspect_running(&self, name: &str) -> Result<Option<serde_json::Value>> {
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
        let running = first
            .pointer("/State/Running")
            .and_then(|value| value.as_bool())
            .unwrap_or(false);
        if running {
            Ok(Some(first.clone()))
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

fn container_name(workload_id: &str) -> String {
    let compact: String = workload_id.chars().filter(|ch| ch.is_ascii_hexdigit()).collect();
    format!("norix-{}", &compact[..compact.len().min(12)])
}

fn normalize_image(value: &str) -> &str {
    value
        .trim()
        .trim_matches('"')
        .trim_start_matches("docker.io/")
        .trim_start_matches("index.docker.io/")
        .trim_start_matches("library/")
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

#[cfg(test)]
mod tests {
    use super::image_matches;

    #[test]
    fn matches_same_image_and_registry_prefix() {
        assert!(image_matches("docker.io/library/nginx:latest", "nginx:latest"));
        assert!(image_matches("ghcr.io/acme/api:1.2.3", "api:1.2.3"));
        assert!(image_matches("ghcr.io/acme/nginx", "nginx"));
    }

    #[test]
    fn does_not_match_other_images_by_substring() {
        assert!(!image_matches("ghcr.io/acme/nginx-prod:latest", "nginx:latest"));
        assert!(!image_matches("ghcr.io/acme/not-nginx:latest", "nginx:latest"));
        assert!(!image_matches("ghcr.io/acme/myapi:1.2.3", "api:1.2.3"));
        assert!(!image_matches("ghcr.io/acme/my-nginx:latest", "nginx:latest"));
    }
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
