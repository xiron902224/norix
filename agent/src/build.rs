use std::path::{Path, PathBuf};
use std::process::Stdio;
use std::sync::{Arc, OnceLock};
use std::time::Duration;

use anyhow::{bail, Context, Result};
use tokio::fs;
use tokio::process::Command;
use tokio::sync::{Mutex, OwnedSemaphorePermit, Semaphore};
use tracing::info;

use crate::client::{DesiredBuild, ObservedBuild};
use crate::gitssrf;
use crate::runtime::Runtime;

/// Default number of builds that run at the same time on one node (#24).
pub const DEFAULT_BUILD_CONCURRENCY: usize = 2;
const MAX_BUILD_CONCURRENCY: usize = 8;
/// Default wall-clock limit for one build, in seconds.
pub const DEFAULT_BUILD_TIMEOUT_SECS: u64 = 3600;
const MIN_BUILD_TIMEOUT_SECS: u64 = 60;
const MAX_BUILD_TIMEOUT_SECS: u64 = 21600;

/// Limits how many builds run at once on this node. Builds beyond the limit wait
/// for a slot and report `queued` while they wait.
pub struct BuildSlots {
    semaphore: Arc<Semaphore>,
    limit: usize,
    timeout: Duration,
}

impl BuildSlots {
    pub fn new(limit: usize, timeout: Duration) -> Self {
        let limit = limit.max(1);
        Self {
            semaphore: Arc::new(Semaphore::new(limit)),
            limit,
            timeout,
        }
    }

    pub fn limit(&self) -> usize {
        self.limit
    }

    pub fn busy(&self) -> usize {
        self.limit - self.semaphore.available_permits()
    }

    pub fn timeout(&self) -> Duration {
        self.timeout
    }

    pub fn try_acquire(&self) -> Option<OwnedSemaphorePermit> {
        self.semaphore.clone().try_acquire_owned().ok()
    }

    pub async fn acquire(&self) -> OwnedSemaphorePermit {
        // The semaphore is never closed, so acquiring cannot fail.
        self.semaphore
            .clone()
            .acquire_owned()
            .await
            .expect("build semaphore is never closed")
    }
}

/// The process-wide build slots, configured from `NORIX_BUILD_CONCURRENCY` and
/// `NORIX_BUILD_TIMEOUT_SECS` on first use.
pub fn slots() -> &'static BuildSlots {
    static SLOTS: OnceLock<BuildSlots> = OnceLock::new();
    SLOTS.get_or_init(|| {
        BuildSlots::new(
            parse_concurrency(std::env::var("NORIX_BUILD_CONCURRENCY").ok().as_deref()),
            parse_timeout(std::env::var("NORIX_BUILD_TIMEOUT_SECS").ok().as_deref()),
        )
    })
}

/// Parses the concurrency. Missing or invalid values use the default. Values are clamped to 1..=8.
pub fn parse_concurrency(raw: Option<&str>) -> usize {
    raw.and_then(|v| v.trim().parse::<usize>().ok())
        .map(|v| v.clamp(1, MAX_BUILD_CONCURRENCY))
        .unwrap_or(DEFAULT_BUILD_CONCURRENCY)
}

/// Parses the timeout in seconds. Values are clamped to 60..=21600.
pub fn parse_timeout(raw: Option<&str>) -> Duration {
    let secs = raw
        .and_then(|v| v.trim().parse::<u64>().ok())
        .map(|v| v.clamp(MIN_BUILD_TIMEOUT_SECS, MAX_BUILD_TIMEOUT_SECS))
        .unwrap_or(DEFAULT_BUILD_TIMEOUT_SECS);
    Duration::from_secs(secs)
}

/// One BuildKit daemon container is shared by all Railpack builds. Starting it is
/// check-then-act, so two builds must not do it at the same time.
fn buildkit_lock() -> &'static Mutex<()> {
    static LOCK: OnceLock<Mutex<()>> = OnceLock::new();
    LOCK.get_or_init(|| Mutex::new(()))
}

impl Runtime {
    /// Runs one build under the timeout. Failures become a `failed` report, never an error.
    pub async fn run_build_with_timeout(&self, build: &DesiredBuild) -> ObservedBuild {
        let limit = slots().timeout();
        let failed = |message: String| ObservedBuild {
            id: build.id.clone(),
            status: "failed".into(),
            image: String::new(),
            message: message.clone(),
            log_lines: vec![message],
        };
        match tokio::time::timeout(limit, self.run_build(build)).await {
            Ok(Ok(value)) => value,
            Ok(Err(error)) => failed(error.to_string()),
            // The future is dropped here. kill_on_drop stops the git and docker children.
            Err(_) => failed(format!("build timed out after {}s", limit.as_secs())),
        }
    }

    async fn run_build(&self, build: &DesiredBuild) -> Result<ObservedBuild> {
        let mut logs = vec![format!("building {} @ {}", build.git_url, build.git_ref)];
        gitssrf::validate_git_url(&build.git_url)?;
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

        let (kind, path, discover_logs) =
            resolve_build_source(&clone_dir, &build.dockerfile_path).await;
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
            "railpack" => {
                logs.push(format!("using Railpack ({path})"));
                self.build_railpack(&clone_dir, build, &mut logs).await?;
            }
            _ => bail!("no Dockerfile, compose, or Railpack source found"),
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

    async fn build_railpack(
        &self,
        clone_dir: &Path,
        build: &DesiredBuild,
        logs: &mut Vec<String>,
    ) -> Result<()> {
        let (ok, which) = run_capture("railpack", &["--version"]).await;
        if !ok {
            bail!(
                "railpack not found in PATH — install from https://railpack.com/installation/ ({which})"
            );
        }
        logs.extend(truncate_lines(&which, 5));

        let host = ensure_buildkit_host(logs).await?;
        logs.push(format!("railpack build --name {} .", build.image_tag));
        let output = Command::new("railpack")
            .args([
                "build",
                "--name",
                &build.image_tag,
                "--progress",
                "plain",
                ".",
            ])
            .current_dir(clone_dir)
            .env("BUILDKIT_HOST", &host)
            .env("DOCKER_BUILDKIT", "1")
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .kill_on_drop(true)
            .output()
            .await
            .context("run railpack")?;
        let mut text = String::from_utf8_lossy(&output.stdout).to_string();
        text.push_str(&String::from_utf8_lossy(&output.stderr));
        logs.extend(truncate_lines(&text, 120));
        if !output.status.success() {
            bail!("railpack build failed");
        }
        Ok(())
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
            .engine_capture(
                &["tag".into(), local_image.clone(), build.image_tag.clone()],
                None,
            )
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

    if let Some(reason) = detect_railpack_source(clone_dir).await {
        logs.push(format!(
            "no Dockerfile/compose; falling back to Railpack for {reason}"
        ));
        return ("railpack".into(), reason, logs);
    }

    logs.push("no Dockerfile, compose file, or Railpack-detectable source found in repo".into());
    (String::new(), String::new(), logs)
}

async fn detect_railpack_source(clone_dir: &Path) -> Option<String> {
    const MARKERS: &[(&str, &str)] = &[
        ("package.json", "Node.js (package.json)"),
        ("go.mod", "Go (go.mod)"),
        ("pyproject.toml", "Python (pyproject.toml)"),
        ("requirements.txt", "Python (requirements.txt)"),
        ("Pipfile", "Python (Pipfile)"),
        ("index.html", "static site (index.html)"),
    ];
    for (file, label) in MARKERS {
        if clone_dir.join(file).is_file() {
            return Some((*label).to_string());
        }
    }
    None
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

async fn ensure_buildkit_host(logs: &mut Vec<String>) -> Result<String> {
    if let Ok(host) = std::env::var("BUILDKIT_HOST") {
        let host = host.trim().to_string();
        if !host.is_empty() {
            logs.push(format!("using BUILDKIT_HOST={host}"));
            return Ok(host);
        }
    }
    const NAME: &str = "norix-buildkit";
    let _guard = buildkit_lock().lock().await;
    let (ok, out) = run_capture("docker", &["inspect", "-f", "{{.State.Running}}", NAME]).await;
    let running = ok && out.trim() == "true";
    if !running {
        logs.push(format!("starting BuildKit container {NAME}"));
        let _ = run_capture("docker", &["rm", "-f", NAME]).await;
        let mut args = vec!["run", "--rm", "--privileged", "-d", "--name", NAME];
        // Optional limits for the shared BuildKit daemon, for example NORIX_BUILDKIT_CPUS=2
        // and NORIX_BUILDKIT_MEMORY=4g. They apply when the container starts. docker build
        // runs in the Docker daemon and is not limited here.
        let cpus = std::env::var("NORIX_BUILDKIT_CPUS").unwrap_or_default();
        let memory = std::env::var("NORIX_BUILDKIT_MEMORY").unwrap_or_default();
        if !cpus.trim().is_empty() {
            args.extend(["--cpus", cpus.trim()]);
        }
        if !memory.trim().is_empty() {
            args.extend(["--memory", memory.trim()]);
        }
        args.push("moby/buildkit:buildx-stable-1");
        let (started, start_out) = run_capture("docker", &args).await;
        if !started {
            bail!("start buildkit failed: {start_out}");
        }
        tokio::time::sleep(std::time::Duration::from_secs(2)).await;
    } else {
        logs.push(format!("reusing BuildKit container {NAME}"));
    }
    Ok(format!("docker-container://{NAME}"))
}

async fn run_capture(program: &str, args: &[&str]) -> (bool, String) {
    let output = Command::new(program)
        .args(args)
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .kill_on_drop(true)
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

#[cfg(test)]
mod slot_tests {
    use super::*;

    #[test]
    fn concurrency_defaults_and_clamps() {
        assert_eq!(parse_concurrency(None), DEFAULT_BUILD_CONCURRENCY);
        assert_eq!(parse_concurrency(Some("nope")), DEFAULT_BUILD_CONCURRENCY);
        assert_eq!(parse_concurrency(Some("0")), 1);
        assert_eq!(parse_concurrency(Some(" 4 ")), 4);
        assert_eq!(parse_concurrency(Some("99")), MAX_BUILD_CONCURRENCY);
    }

    #[test]
    fn timeout_defaults_and_clamps() {
        assert_eq!(
            parse_timeout(None),
            Duration::from_secs(DEFAULT_BUILD_TIMEOUT_SECS)
        );
        assert_eq!(
            parse_timeout(Some("5")),
            Duration::from_secs(MIN_BUILD_TIMEOUT_SECS)
        );
        assert_eq!(parse_timeout(Some("900")), Duration::from_secs(900));
        assert_eq!(
            parse_timeout(Some("999999")),
            Duration::from_secs(MAX_BUILD_TIMEOUT_SECS)
        );
    }

    #[test]
    fn slots_block_past_the_limit_and_release_on_drop() {
        let slots = BuildSlots::new(2, Duration::from_secs(60));
        let first = slots.try_acquire().expect("first slot");
        let second = slots.try_acquire().expect("second slot");
        assert_eq!(slots.busy(), 2);
        assert!(slots.try_acquire().is_none(), "third build must wait");
        drop(first);
        assert_eq!(slots.busy(), 1);
        assert!(slots.try_acquire().is_some());
        drop(second);
    }

    #[test]
    fn zero_limit_is_raised_to_one() {
        let slots = BuildSlots::new(0, Duration::from_secs(60));
        assert_eq!(slots.limit(), 1);
    }
}
