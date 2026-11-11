use std::path::{Path, PathBuf};
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

        let (kind, path, discover_logs) = resolve_build_source(&clone_dir, &build.dockerfile_path).await;
        logs.extend(discover_logs);
        match kind.as_str() {
            "dockerfile" => {
                logs.push(format!("using Dockerfile {path}"));
                self.build_dockerfile(&clone_dir, &path, build, &mut logs)
                    .await?;
            }
            "compose" => {
                logs.push(format!("using compose file {path}"));
                self.build_compose(&clone_dir, &path, build, &mut logs)
                    .await?;
            }
            _ => bail!("no Dockerfile or compose file found"),
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

    async fn build_dockerfile(
        &self,
        clone_dir: &Path,
        dockerfile: &str,
        build: &DesiredBuild,
        logs: &mut Vec<String>,
    ) -> Result<()> {
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
        let (ok, out) = self.engine_capture(&args, Some(clone_dir)).await?;
        logs.extend(truncate_lines(&out, 80));
        if !ok {
            bail!("docker build failed");
        }
        Ok(())
    }

    async fn build_compose(
        &self,
        clone_dir: &Path,
        compose_file: &str,
        build: &DesiredBuild,
        logs: &mut Vec<String>,
    ) -> Result<()> {
        let project = format!("norix{}", sanitize_project(&build.id));
        let services_args = vec![
            "compose".into(),
            "-p".into(),
            project.clone(),
            "-f".into(),
            compose_file.to_string(),
            "config".into(),
            "--services".into(),
        ];
        let (ok, out) = self.engine_capture(&services_args, Some(clone_dir)).await?;
        logs.extend(truncate_lines(&out, 20));
        let services_text = if ok {
            out
        } else {
            let (ok2, out2) = run_capture(
                "docker-compose",
                &["-p", &project, "-f", compose_file, "config", "--services"],
            )
            .await;
            logs.extend(truncate_lines(&out2, 20));
            if !ok2 {
                bail!("compose config failed");
            }
            out2
        };
        let services: Vec<&str> = services_text.split_whitespace().collect();
        if services.is_empty() {
            bail!("compose file has no services");
        }
        let service = pick_compose_service(&services).to_string();
        logs.push(format!("building compose service {service}"));

        let build_args = vec![
            "compose".into(),
            "-p".into(),
            project.clone(),
            "-f".into(),
            compose_file.to_string(),
            "build".into(),
            service.clone(),
        ];
        let (ok, out) = self.engine_capture(&build_args, Some(clone_dir)).await?;
        logs.extend(truncate_lines(&out, 80));
        if !ok {
            logs.push("compose build failed; trying pull for image-based services".into());
            let pull_args = vec![
                "compose".into(),
                "-p".into(),
                project.clone(),
                "-f".into(),
                compose_file.to_string(),
                "pull".into(),
                service.clone(),
            ];
            let (_, pull_out) = self.engine_capture(&pull_args, Some(clone_dir)).await?;
            logs.extend(truncate_lines(&pull_out, 40));
        }

        let local_image = format!("{project}-{service}");
        let (ok, out) = self
            .engine_capture(&["tag".into(), local_image.clone(), build.image_tag.clone()], None)
            .await?;
        logs.extend(truncate_lines(&out, 10));
        if !ok {
            let alt = format!("{project}_{service}");
            let (ok2, out2) = self
                .engine_capture(&["tag".into(), alt.clone(), build.image_tag.clone()], None)
                .await?;
            logs.extend(truncate_lines(&out2, 10));
            if !ok2 {
                let img_args = vec![
                    "compose".into(),
                    "-p".into(),
                    project,
                    "-f".into(),
                    compose_file.to_string(),
                    "config".into(),
                    "--images".into(),
                    service,
                ];
                let (ok3, out3) = self.engine_capture(&img_args, Some(clone_dir)).await?;
                logs.extend(truncate_lines(&out3, 10));
                if !ok3 {
                    bail!("compose image tag failed for {local_image} / {alt}");
                }
                let image = out3.split_whitespace().next().unwrap_or_default();
                if image.is_empty() {
                    bail!("compose produced no image");
                }
                let (ok4, out4) = self
                    .engine_capture(&["tag".into(), image.into(), build.image_tag.clone()], None)
                    .await?;
                logs.extend(truncate_lines(&out4, 10));
                if !ok4 {
                    bail!("tag {image} -> {} failed", build.image_tag);
                }
            }
        }
        Ok(())
    }
}

async fn resolve_build_source(clone_dir: &Path, hint: &str) -> (String, String, Vec<String>) {
    let mut logs = Vec::new();
    let hint = hint.trim();
    if !hint.is_empty() {
        let candidate = clone_dir.join(hint);
        if candidate.is_file() {
            if is_compose_name(hint) {
                return (
                    "compose".into(),
                    hint.to_string(),
                    vec![format!("hint {hint} is a compose file")],
                );
            }
            return (
                "dockerfile".into(),
                hint.to_string(),
                vec![format!("using hinted Dockerfile {hint}")],
            );
        }
        logs.push(format!("hinted path {hint} not found; auto-detecting"));
    }

    for name in [
        "Dockerfile",
        "dockerfile",
        "Containerfile",
        "docker/Dockerfile",
        "build/Dockerfile",
        ".docker/Dockerfile",
    ] {
        if clone_dir.join(name).is_file() {
            logs.push(format!("auto-detected Dockerfile {name}"));
            return ("dockerfile".into(), name.to_string(), logs);
        }
    }

    if let Ok(mut entries) = fs::read_dir(clone_dir).await {
        while let Ok(Some(entry)) = entries.next_entry().await {
            let ft = match entry.file_type().await {
                Ok(v) => v,
                Err(_) => continue,
            };
            if !ft.is_dir() {
                continue;
            }
            let name = entry.file_name();
            let name = name.to_string_lossy();
            if name == ".git" || name == "node_modules" || name == "vendor" {
                continue;
            }
            for file in ["Dockerfile", "dockerfile", "Containerfile"] {
                let rel = format!("{name}/{file}");
                if clone_dir.join(&rel).is_file() {
                    logs.push(format!("auto-detected Dockerfile {rel}"));
                    return ("dockerfile".into(), rel, logs);
                }
            }
        }
    }

    for name in [
        "compose.yaml",
        "compose.yml",
        "docker-compose.yml",
        "docker-compose.yaml",
    ] {
        if clone_dir.join(name).is_file() {
            logs.push(format!("auto-detected compose {name}"));
            return ("compose".into(), name.to_string(), logs);
        }
    }

    logs.push("no Dockerfile or compose file found in repo".into());
    (String::new(), String::new(), logs)
}

fn is_compose_name(name: &str) -> bool {
    let base = Path::new(name)
        .file_name()
        .and_then(|v| v.to_str())
        .unwrap_or(name)
        .to_ascii_lowercase();
    matches!(
        base.as_str(),
        "compose.yaml" | "compose.yml" | "docker-compose.yml" | "docker-compose.yaml"
    ) || (base.contains("compose") && (base.ends_with(".yml") || base.ends_with(".yaml")))
}

fn pick_compose_service<'a>(services: &[&'a str]) -> &'a str {
    const PREFERRED: &[&str] = &["web", "app", "frontend", "api", "server", "backend"];
    for want in PREFERRED {
        if let Some(found) = services.iter().find(|s| s.eq_ignore_ascii_case(want)) {
            return found;
        }
    }
    services[0]
}

fn sanitize_project(id: &str) -> String {
    let cleaned: String = id.chars().filter(|c| c.is_ascii_hexdigit()).collect();
    cleaned.chars().take(8).collect()
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
