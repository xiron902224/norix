use std::path::PathBuf;
use std::process::Stdio;

use anyhow::{bail, Context, Result};
use tokio::fs;
use tokio::process::Command;
use tracing::info;

use crate::client::{DesiredBuild, ObservedBuild};
use crate::runtime::Runtime;

impl Runtime {
    pub async fn run_builds(&self, builds: &[DesiredBuild]) -> Vec<ObservedBuild> {
        let mut observed = Vec::with_capacity(builds.len());
        for build in builds {
            match self.run_build(build).await {
                Ok(value) => observed.push(value),
                Err(error) => observed.push(ObservedBuild {
                    id: build.id.clone(),
                    status: "failed".into(),
                    image: String::new(),
                    message: error.to_string(),
                    log_lines: vec![error.to_string()],
                }),
            }
        }
        observed
    }

    async fn run_build(&self, build: &DesiredBuild) -> Result<ObservedBuild> {
        let mut logs = vec![format!("building {} @ {}", build.git_url, build.git_ref)];
        let workdir = tempfile_dir().await?;
        let clone_dir = workdir.join("src");
        let ref_name = if build.git_ref.is_empty() {
            "main"
        } else {
            build.git_ref.as_str()
        };

        let (ok, out) = run_capture(
            "git",
            &[
                "clone",
                "--depth",
                "1",
                "--branch",
                ref_name,
                &build.git_url,
                clone_dir.to_str().unwrap(),
            ],
        )
        .await;
        logs.extend(truncate_lines(&out, 40));
        if !ok {
            let _ = fs::remove_dir_all(&clone_dir).await;
            let (ok2, out2) = run_capture(
                "git",
                &[
                    "clone",
                    "--depth",
                    "1",
                    &build.git_url,
                    clone_dir.to_str().unwrap(),
                ],
            )
            .await;
            logs.extend(truncate_lines(&out2, 40));
            if !ok2 {
                bail!("git clone failed");
            }
        }

        let dockerfile = if build.dockerfile_path.is_empty() {
            "Dockerfile"
        } else {
            build.dockerfile_path.as_str()
        };
        let dockerfile_path = clone_dir.join(dockerfile);
        if !dockerfile_path.exists() {
            bail!("dockerfile not found at {dockerfile}");
        }

        let mut args = vec![
            "build".to_string(),
            "-f".to_string(),
            dockerfile.to_string(),
            "-t".to_string(),
            build.image_tag.clone(),
        ];
        for (key, value) in &build.build_args {
            args.push("--build-arg".into());
            args.push(format!("{key}={value}"));
        }
        args.push(".".into());

        info!(image = %build.image_tag, "docker build");
        let (ok, out) = self.engine_capture(&args, Some(&clone_dir)).await?;
        logs.extend(truncate_lines(&out, 80));
        if !ok {
            bail!("docker build failed");
        }

        if !build.registry.is_empty() && build.image_tag.contains(&build.registry) {
            logs.push(format!("pushing {}", build.image_tag));
            let (ok, out) = self
                .engine_capture(&["push".into(), build.image_tag.clone()], None)
                .await?;
            logs.extend(truncate_lines(&out, 40));
            if !ok {
                logs.push("push failed; continuing with local image tag".into());
            }
        }

        logs.push("build succeeded".into());
        let _ = fs::remove_dir_all(&workdir).await;
        Ok(ObservedBuild {
            id: build.id.clone(),
            status: "succeeded".into(),
            image: build.image_tag.clone(),
            message: String::new(),
            log_lines: logs,
        })
    }
}

async fn tempfile_dir() -> Result<PathBuf> {
    let path = std::env::temp_dir().join(format!("norix-build-{}", uuid_like()));
    fs::create_dir_all(&path)
        .await
        .with_context(|| format!("create {}", path.display()))?;
    Ok(path)
}

fn uuid_like() -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_nanos())
        .unwrap_or(0);
    format!("{nanos:x}")
}

async fn run_capture(program: &str, args: &[&str]) -> (bool, String) {
    let output = Command::new(program)
        .args(args)
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .output()
        .await;
    match output {
        Ok(out) => {
            let mut text = String::from_utf8_lossy(&out.stdout).to_string();
            text.push_str(&String::from_utf8_lossy(&out.stderr));
            (out.status.success(), text)
        }
        Err(error) => (false, error.to_string()),
    }
}

fn truncate_lines(output: &str, max: usize) -> Vec<String> {
    output
        .lines()
        .map(str::trim)
        .filter(|line| !line.is_empty())
        .take(max)
        .map(str::to_string)
        .collect()
}
