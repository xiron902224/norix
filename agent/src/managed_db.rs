use anyhow::{bail, Result};
use tracing::info;

use crate::client::{DesiredManagedDatabase, ObservedManagedDatabase};
use crate::runtime::Runtime;

impl Runtime {
    pub async fn reconcile_managed_databases(
        &self,
        desired: &[DesiredManagedDatabase],
    ) -> Vec<ObservedManagedDatabase> {
        let mut out = Vec::with_capacity(desired.len());
        for item in desired {
            match self.reconcile_managed_one(item).await {
                Ok(obs) => out.push(obs),
                Err(error) => out.push(ObservedManagedDatabase {
                    id: item.id.clone(),
                    status: "failed".into(),
                    container_id: String::new(),
                    message: error.to_string(),
                    host_port: item.host_port,
                }),
            }
        }
        out
    }

    async fn reconcile_managed_one(
        &self,
        desired: &DesiredManagedDatabase,
    ) -> Result<ObservedManagedDatabase> {
        let name = format!(
            "norix-mdb-{}",
            desired
                .id
                .replace('-', "")
                .chars()
                .take(12)
                .collect::<String>()
        );
        if desired.action == "stop" {
            let _ = self
                .engine_capture(&["rm".into(), "-f".into(), name.clone()], None)
                .await?;
            return Ok(ObservedManagedDatabase {
                id: desired.id.clone(),
                status: "stopped".into(),
                container_id: String::new(),
                message: String::new(),
                host_port: desired.host_port,
            });
        }

        if !desired.volume_name.is_empty() {
            let _ = self
                .engine_capture(
                    &[
                        "volume".into(),
                        "create".into(),
                        desired.volume_name.clone(),
                    ],
                    None,
                )
                .await;
        }

        let (running, inspect) = self
            .engine_capture(
                &[
                    "inspect".into(),
                    "-f".into(),
                    "{{.State.Running}} {{.Id}}".into(),
                    name.clone(),
                ],
                None,
            )
            .await?;
        if running && inspect.trim().starts_with("true") {
            let cid = inspect
                .split_whitespace()
                .nth(1)
                .unwrap_or_default()
                .to_string();
            return Ok(ObservedManagedDatabase {
                id: desired.id.clone(),
                status: "ready".into(),
                container_id: cid,
                message: String::new(),
                host_port: desired.host_port,
            });
        }

        let _ = self
            .engine_capture(&["rm".into(), "-f".into(), name.clone()], None)
            .await;

        let (ok_pull, pull_out) = self
            .engine_capture(&["pull".into(), desired.image.clone()], None)
            .await?;
        if !ok_pull {
            info!(image = %desired.image, out = %pull_out, "managed db pull soft-failed; trying run");
        }

        let mut args = vec![
            "run".into(),
            "-d".into(),
            "--name".into(),
            name.clone(),
            "--restart".into(),
            "unless-stopped".into(),
            "--label".into(),
            format!("norix.managed_database_id={}", desired.id),
            "-p".into(),
            format!("{}:{}", desired.host_port, desired.container_port),
        ];
        for (k, v) in &desired.env_vars {
            args.push("-e".into());
            args.push(format!("{k}={v}"));
        }
        if !desired.volume_name.is_empty() && !desired.volume_mount_path.is_empty() {
            args.push("-v".into());
            args.push(format!(
                "{}:{}",
                desired.volume_name, desired.volume_mount_path
            ));
        }
        args.push(desired.image.clone());
        for c in &desired.command {
            args.push(c.clone());
        }

        let (ok, out) = self.engine_capture(&args, None).await?;
        if !ok {
            bail!("docker run managed db failed: {}", out.trim());
        }
        let cid = out.trim().chars().take(64).collect::<String>();
        // Brief wait so engines can accept connections before we mark ready.
        tokio::time::sleep(std::time::Duration::from_secs(2)).await;
        Ok(ObservedManagedDatabase {
            id: desired.id.clone(),
            status: "ready".into(),
            container_id: cid,
            message: String::new(),
            host_port: desired.host_port,
        })
    }
}
