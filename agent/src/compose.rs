//! Compose stacks applied as a unit with `docker compose` (#143).
//!
//! Each stack lives in `<workdir>/stacks/<id>/`: `src/` is the git checkout
//! (kept between deploys so relative bind mounts keep their data),
//! `norix.override.json` adds Norix labels, `generation` is the last applied
//! generation and `last_error` the last failed apply.

use std::collections::{HashMap, HashSet};
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use anyhow::{bail, Context, Result};
use serde_json::{json, Value};
use tokio::fs;
use tokio::process::Command;
use tracing::{info, warn};

use crate::client::{
    AgentClient, DesiredComposeStack, NodeState, ObservedComposeLogLine, ObservedComposeService,
    ObservedComposeStack,
};
use crate::gitssrf;

const DEPLOY_LOG_SERVICE: &str = "_deploy";
const DEFAULT_UP_TIMEOUT_SECS: u64 = 3600;
const OBSERVE_TIMEOUT: Duration = Duration::from_secs(30);
const MAX_LOG_LINES: usize = 200;

#[derive(Clone)]
pub struct ComposeStacks {
    bin: &'static str,
    root: PathBuf,
    inflight: Arc<Mutex<HashSet<String>>>,
    log_cursor: Arc<Mutex<HashMap<String, String>>>,
}

impl ComposeStacks {
    pub fn new(bin: &'static str, workdir: &Path) -> Self {
        // Compose runs with cwd inside the checkout, so every path we pass must be absolute.
        let workdir = if workdir.is_absolute() {
            workdir.to_path_buf()
        } else {
            std::env::current_dir()
                .map(|cwd| cwd.join(workdir))
                .unwrap_or_else(|_| workdir.to_path_buf())
        };
        Self {
            bin,
            root: workdir.join("stacks"),
            inflight: Arc::new(Mutex::new(HashSet::new())),
            log_cursor: Arc::new(Mutex::new(HashMap::new())),
        }
    }

    /// Starts one background task per stack that is not already being handled,
    /// so a long `up --build` never blocks heartbeats.
    pub fn reconcile(
        &self,
        client: &AgentClient,
        state: &NodeState,
        desired: &[DesiredComposeStack],
    ) {
        for stack in desired {
            if let Err(error) = validate_stack_identity(stack) {
                warn!(%error, stack_id = %stack.id, "skip invalid compose stack");
                continue;
            }
            {
                let mut inflight = self.inflight.lock().expect("compose inflight lock");
                if !inflight.insert(stack.id.clone()) {
                    continue;
                }
            }
            let this = self.clone();
            let client = client.clone();
            let state = state.clone();
            let stack = stack.clone();
            tokio::spawn(async move {
                let observed = this.handle(&client, &state, &stack).await;
                if let Err(error) = client.report_compose_stack(&state, &observed).await {
                    warn!(%error, stack_id = %stack.id, "report compose stack failed");
                }
                this.inflight
                    .lock()
                    .expect("compose inflight lock")
                    .remove(&stack.id);
            });
        }
    }

    async fn handle(
        &self,
        client: &AgentClient,
        state: &NodeState,
        stack: &DesiredComposeStack,
    ) -> ObservedComposeStack {
        let dir = self.root.join(&stack.id);
        let applied = read_generation(&dir).await;
        if stack.generation > applied {
            let _ = client
                .report_compose_stack(
                    state,
                    &ObservedComposeStack {
                        id: stack.id.clone(),
                        status: "deploying".into(),
                        observed_generation: applied,
                        message: format!("applying {} ({})", stack.action, stack.compose_path),
                        services: vec![],
                        log_lines: vec![],
                    },
                )
                .await;
            let mut logs = Vec::new();
            let result = self.apply(stack, &dir, &mut logs).await;
            let log_lines = logs
                .into_iter()
                .rev()
                .take(MAX_LOG_LINES)
                .collect::<Vec<_>>()
                .into_iter()
                .rev()
                .map(|line| ObservedComposeLogLine {
                    service: DEPLOY_LOG_SERVICE.into(),
                    line,
                })
                .collect::<Vec<_>>();
            if stack.action == "down" {
                return match result {
                    Ok(()) => ObservedComposeStack {
                        id: stack.id.clone(),
                        status: "removed".into(),
                        observed_generation: stack.generation,
                        message: String::new(),
                        services: vec![],
                        log_lines,
                    },
                    Err(error) => ObservedComposeStack {
                        id: stack.id.clone(),
                        status: "failed".into(),
                        observed_generation: applied,
                        message: format!("{error:#}"),
                        services: vec![],
                        log_lines,
                    },
                };
            }
            let _ = fs::create_dir_all(&dir).await;
            match &result {
                Ok(()) => {
                    let _ = fs::remove_file(dir.join("last_error")).await;
                }
                Err(error) => {
                    let _ = fs::write(dir.join("last_error"), format!("{error:#}")).await;
                }
            }
            // A failed apply still counts as handled; the user retries with Redeploy.
            let _ = fs::write(dir.join("generation"), stack.generation.to_string()).await;
            info!(stack_id = %stack.id, action = %stack.action, ok = result.is_ok(), "compose stack applied");
            let mut observed = self.observe(stack, &dir).await;
            observed.log_lines.splice(0..0, log_lines);
            return observed;
        }
        if stack.action == "down" {
            return ObservedComposeStack {
                id: stack.id.clone(),
                status: "removed".into(),
                observed_generation: stack.generation,
                ..Default::default()
            };
        }
        self.observe(stack, &dir).await
    }

    async fn apply(
        &self,
        stack: &DesiredComposeStack,
        dir: &Path,
        logs: &mut Vec<String>,
    ) -> Result<()> {
        match stack.action.as_str() {
            "up" => self.up(stack, dir, logs).await,
            "stop" => {
                let (ok, out) = self
                    .compose(
                        &stack.compose_project,
                        None,
                        &["stop"],
                        None,
                        OBSERVE_TIMEOUT * 4,
                    )
                    .await?;
                logs.extend(tail_lines(&out, MAX_LOG_LINES));
                if !ok {
                    bail!("docker compose stop failed");
                }
                Ok(())
            }
            "down" => {
                let (ok, out) = self
                    .compose(
                        &stack.compose_project,
                        None,
                        &["down", "--remove-orphans"],
                        None,
                        OBSERVE_TIMEOUT * 4,
                    )
                    .await?;
                logs.extend(tail_lines(&out, MAX_LOG_LINES));
                if !ok {
                    bail!("docker compose down failed");
                }
                let _ = fs::remove_dir_all(dir).await;
                self.log_cursor
                    .lock()
                    .expect("compose log cursor lock")
                    .remove(&stack.id);
                Ok(())
            }
            other => bail!("unsupported compose action {other}"),
        }
    }

    async fn up(
        &self,
        stack: &DesiredComposeStack,
        dir: &Path,
        logs: &mut Vec<String>,
    ) -> Result<()> {
        gitssrf::validate_git_url(&stack.git_url)?;
        fs::create_dir_all(dir).await.context("create stack dir")?;
        let src = dir.join("src");
        sync_checkout(&stack.git_url, &stack.git_ref, &src, logs).await?;
        let compose_file = safe_compose_file(&src, &stack.compose_path)?;
        let project_dir = compose_file.parent().unwrap_or(&src).to_path_buf();

        let (ok, out) = self
            .compose(
                &stack.compose_project,
                Some(&[compose_file.as_path()]),
                &["config", "--services"],
                Some(&project_dir),
                OBSERVE_TIMEOUT,
            )
            .await?;
        if !ok {
            logs.extend(tail_lines(&out, 40));
            bail!("docker compose config failed: {}", last_line(&out));
        }
        let services: Vec<String> = out
            .lines()
            .map(str::trim)
            .filter(|s| !s.is_empty())
            .map(String::from)
            .collect();
        if services.is_empty() {
            bail!("compose file {} has no services", stack.compose_path);
        }
        let override_path = dir.join("norix.override.json");
        let override_doc = build_override(&services, &stack.id, &stack.project_id);
        fs::write(&override_path, serde_json::to_vec_pretty(&override_doc)?)
            .await
            .context("write compose override")?;

        logs.push(format!(
            "docker compose -p {} up -d --build --remove-orphans ({} services)",
            stack.compose_project,
            services.len()
        ));
        let (ok, out) = self
            .compose(
                &stack.compose_project,
                Some(&[compose_file.as_path(), override_path.as_path()]),
                &["up", "-d", "--build", "--remove-orphans"],
                Some(&project_dir),
                Duration::from_secs(up_timeout_secs()),
            )
            .await?;
        logs.extend(tail_lines(&out, MAX_LOG_LINES));
        if !ok {
            bail!("docker compose up failed: {}", last_line(&out));
        }
        Ok(())
    }

    async fn observe(&self, stack: &DesiredComposeStack, dir: &Path) -> ObservedComposeStack {
        let applied = read_generation(dir).await;
        let mut observed = ObservedComposeStack {
            id: stack.id.clone(),
            observed_generation: applied,
            ..Default::default()
        };
        match self
            .compose(
                &stack.compose_project,
                None,
                &["ps", "-a", "--format", "json"],
                None,
                OBSERVE_TIMEOUT,
            )
            .await
        {
            Ok((true, out)) => observed.services = parse_ps_output(&out),
            Ok((false, out)) => observed.message = last_line(&out),
            Err(error) => observed.message = format!("{error:#}"),
        }
        observed.status = derive_status(&stack.action, &observed.services).into();
        if let Ok(error) = fs::read_to_string(dir.join("last_error")).await {
            if !error.trim().is_empty() {
                observed.status = "failed".into();
                observed.message = error.trim().to_string();
            }
        }
        observed.log_lines = self.collect_logs(stack).await;
        observed
    }

    async fn collect_logs(&self, stack: &DesiredComposeStack) -> Vec<ObservedComposeLogLine> {
        let cursor = self
            .log_cursor
            .lock()
            .expect("compose log cursor lock")
            .get(&stack.id)
            .cloned();
        let tail = MAX_LOG_LINES.to_string();
        let mut args = vec![
            "logs",
            "--no-color",
            "--timestamps",
            "--tail",
            tail.as_str(),
        ];
        if let Some(since) = cursor.as_deref() {
            args.push("--since");
            args.push(since);
        }
        let Ok((true, out)) = self
            .compose(&stack.compose_project, None, &args, None, OBSERVE_TIMEOUT)
            .await
        else {
            return vec![];
        };
        let mut lines = Vec::new();
        let mut newest = cursor.clone();
        for raw in out.lines() {
            let Some(parsed) = parse_log_line(raw, &stack.compose_project) else {
                continue;
            };
            if let (Some(ts), Some(prev)) = (parsed.timestamp.as_deref(), cursor.as_deref()) {
                if ts <= prev {
                    continue;
                }
            }
            if let Some(ts) = parsed.timestamp.clone() {
                if newest.as_deref().map(|n| ts.as_str() > n).unwrap_or(true) {
                    newest = Some(ts);
                }
            }
            lines.push(ObservedComposeLogLine {
                service: parsed.service,
                line: parsed.message,
            });
        }
        if let Some(ts) = newest {
            self.log_cursor
                .lock()
                .expect("compose log cursor lock")
                .insert(stack.id.clone(), ts);
        }
        if lines.len() > MAX_LOG_LINES {
            lines.drain(0..lines.len() - MAX_LOG_LINES);
        }
        lines
    }

    async fn compose(
        &self,
        project: &str,
        files: Option<&[&Path]>,
        args: &[&str],
        cwd: Option<&Path>,
        timeout: Duration,
    ) -> Result<(bool, String)> {
        let mut command = Command::new(self.bin);
        command.arg("compose").arg("-p").arg(project);
        if let Some(files) = files {
            for file in files {
                command.arg("-f").arg(file);
            }
        }
        command.args(args);
        if let Some(dir) = cwd {
            command.current_dir(dir);
        }
        command.kill_on_drop(true);
        let output = tokio::time::timeout(timeout, command.output())
            .await
            .with_context(|| {
                format!(
                    "{} compose {} timed out after {:?}",
                    self.bin,
                    args.join(" "),
                    timeout
                )
            })?
            .with_context(|| format!("{} compose {}", self.bin, args.join(" ")))?;
        let mut text = String::from_utf8_lossy(&output.stdout).to_string();
        let stderr = String::from_utf8_lossy(&output.stderr);
        if !stderr.trim().is_empty() {
            if !text.is_empty() && !text.ends_with('\n') {
                text.push('\n');
            }
            text.push_str(&stderr);
        }
        Ok((output.status.success(), text))
    }
}

fn up_timeout_secs() -> u64 {
    std::env::var("NORIX_COMPOSE_UP_TIMEOUT_SECS")
        .ok()
        .and_then(|v| v.trim().parse::<u64>().ok())
        .filter(|v| (60..=6 * 3600).contains(v))
        .unwrap_or(DEFAULT_UP_TIMEOUT_SECS)
}

async fn read_generation(dir: &Path) -> i64 {
    fs::read_to_string(dir.join("generation"))
        .await
        .ok()
        .and_then(|v| v.trim().parse().ok())
        .unwrap_or(0)
}

async fn sync_checkout(
    git_url: &str,
    git_ref: &str,
    src: &Path,
    logs: &mut Vec<String>,
) -> Result<()> {
    let git_ref = if git_ref.trim().is_empty() {
        "main"
    } else {
        git_ref.trim()
    };
    if git_ref.starts_with('-') {
        bail!("invalid git ref {git_ref}");
    }
    if src.join(".git").is_dir() {
        let src_str = src.to_string_lossy().to_string();
        let (ok, out) = run_git(&["-C", &src_str, "remote", "set-url", "origin", git_url]).await;
        logs.extend(tail_lines(&out, 10));
        if ok {
            let (ok, out) =
                run_git(&["-C", &src_str, "fetch", "--depth", "1", "origin", git_ref]).await;
            logs.extend(tail_lines(&out, 20));
            if ok {
                let (ok, out) = run_git(&["-C", &src_str, "reset", "--hard", "FETCH_HEAD"]).await;
                logs.extend(tail_lines(&out, 10));
                if ok {
                    return Ok(());
                }
            }
        }
        logs.push("fetch failed; recloning".into());
        let _ = fs::remove_dir_all(src).await;
    }
    let src_str = src.to_string_lossy().to_string();
    let (ok, out) = run_git(&[
        "clone", "--depth", "1", "--branch", git_ref, git_url, &src_str,
    ])
    .await;
    logs.extend(tail_lines(&out, 20));
    if ok {
        return Ok(());
    }
    let _ = fs::remove_dir_all(src).await;
    let (ok, out) = run_git(&["clone", "--depth", "1", git_url, &src_str]).await;
    logs.extend(tail_lines(&out, 20));
    if !ok {
        bail!("git clone failed: {}", last_line(&out));
    }
    Ok(())
}

async fn run_git(args: &[&str]) -> (bool, String) {
    let output = Command::new("git")
        .args(args)
        .env("GIT_TERMINAL_PROMPT", "0")
        .kill_on_drop(true)
        .output();
    match tokio::time::timeout(Duration::from_secs(600), output).await {
        Ok(Ok(output)) => {
            let mut text = String::from_utf8_lossy(&output.stdout).to_string();
            text.push_str(&String::from_utf8_lossy(&output.stderr));
            (output.status.success(), redact_userinfo(&text))
        }
        Ok(Err(error)) => (false, error.to_string()),
        Err(_) => (false, "git timed out after 600s".into()),
    }
}

/// Hides `user:token@` in URLs that git echoes back in errors.
fn redact_userinfo(text: &str) -> String {
    let mut out = String::with_capacity(text.len());
    let mut rest = text;
    while let Some(pos) = rest.find("://") {
        let (head, tail) = rest.split_at(pos + 3);
        out.push_str(head);
        let end = tail
            .find(|c: char| c.is_whitespace() || c == '/' || c == '\'' || c == '"')
            .unwrap_or(tail.len());
        let authority = &tail[..end];
        if let Some(at) = authority.rfind('@') {
            out.push_str("***@");
            out.push_str(&authority[at + 1..]);
        } else {
            out.push_str(authority);
        }
        rest = &tail[end..];
    }
    out.push_str(rest);
    out
}

/// Stack ids become directory names and compose projects become docker labels.
pub fn validate_stack_identity(stack: &DesiredComposeStack) -> Result<()> {
    let id_ok = !stack.id.is_empty()
        && stack.id.len() <= 64
        && stack.id.chars().all(|c| c.is_ascii_hexdigit() || c == '-');
    if !id_ok {
        bail!("invalid stack id");
    }
    let project_ok = stack
        .compose_project
        .strip_prefix("norix-")
        .map(|rest| {
            !rest.is_empty()
                && rest
                    .chars()
                    .all(|c| c.is_ascii_lowercase() || c.is_ascii_digit())
        })
        .unwrap_or(false);
    if !project_ok {
        bail!("invalid compose project {:?}", stack.compose_project);
    }
    Ok(())
}

/// Resolves the compose file and refuses anything outside the checkout.
pub fn safe_compose_file(src: &Path, compose_path: &str) -> Result<PathBuf> {
    let rel = compose_path.trim();
    let rel = if rel.is_empty() {
        "docker-compose.yml"
    } else {
        rel
    };
    let rel_path = Path::new(rel);
    if rel_path.is_absolute()
        || rel_path
            .components()
            .any(|c| matches!(c, std::path::Component::ParentDir))
    {
        bail!("compose path must stay inside the repository");
    }
    let candidate = src.join(rel_path);
    let root = src.canonicalize().context("resolve checkout")?;
    let resolved = candidate
        .canonicalize()
        .with_context(|| format!("compose file {rel} not found in repository"))?;
    if !resolved.starts_with(&root) {
        bail!("compose path must stay inside the repository");
    }
    Ok(resolved)
}

/// Override document (JSON is valid YAML for compose) that labels every service.
pub fn build_override(services: &[String], stack_id: &str, project_id: &str) -> Value {
    let mut map = serde_json::Map::new();
    for name in services {
        map.insert(
            name.clone(),
            json!({
                "labels": {
                    "norix.stack_id": stack_id,
                    "norix.project_id": project_id,
                }
            }),
        );
    }
    json!({ "services": map })
}

/// Parses `docker compose ps --format json` (NDJSON in recent releases, a JSON array in older ones).
pub fn parse_ps_output(raw: &str) -> Vec<ObservedComposeService> {
    let trimmed = raw.trim();
    let mut items: Vec<Value> = Vec::new();
    if trimmed.starts_with('[') {
        if let Ok(Value::Array(values)) = serde_json::from_str::<Value>(trimmed) {
            items = values;
        }
    } else {
        for line in trimmed.lines() {
            if let Ok(value) = serde_json::from_str::<Value>(line.trim()) {
                items.push(value);
            }
        }
    }
    let mut out: Vec<ObservedComposeService> = items
        .iter()
        .filter_map(|item| {
            let name = item.get("Service")?.as_str()?.to_string();
            let state = item
                .get("State")
                .and_then(Value::as_str)
                .unwrap_or("")
                .to_ascii_lowercase();
            let health = item
                .get("Health")
                .and_then(Value::as_str)
                .unwrap_or("")
                .to_ascii_lowercase();
            let exit_code = item.get("ExitCode").and_then(Value::as_i64).unwrap_or(0) as i32;
            let image = item
                .get("Image")
                .and_then(Value::as_str)
                .unwrap_or("")
                .to_string();
            let ports = publishers_to_string(item.get("Publishers"))
                .or_else(|| item.get("Ports").and_then(Value::as_str).map(String::from))
                .unwrap_or_default();
            Some(ObservedComposeService {
                name,
                state,
                health,
                exit_code,
                image,
                ports,
            })
        })
        .collect();
    out.sort_by(|a, b| a.name.cmp(&b.name));
    out
}

fn publishers_to_string(value: Option<&Value>) -> Option<String> {
    let items = value?.as_array()?;
    let mut parts: Vec<String> = items
        .iter()
        .filter_map(|p| {
            let published = p.get("PublishedPort").and_then(Value::as_i64).unwrap_or(0);
            let target = p.get("TargetPort").and_then(Value::as_i64).unwrap_or(0);
            let proto = p.get("Protocol").and_then(Value::as_str).unwrap_or("tcp");
            if target == 0 {
                return None;
            }
            Some(if published > 0 {
                format!("{published}->{target}/{proto}")
            } else {
                format!("{target}/{proto}")
            })
        })
        .collect();
    parts.sort();
    parts.dedup();
    Some(parts.join(", "))
}

/// Aggregates service states. Init jobs that exited 0 count as healthy.
pub fn derive_status(action: &str, services: &[ObservedComposeService]) -> &'static str {
    if action == "stop" {
        return if services
            .iter()
            .any(|s| s.state == "running" || s.state == "restarting")
        {
            "deploying"
        } else {
            "stopped"
        };
    }
    if services.is_empty() {
        return "stopped";
    }
    let mut starting = false;
    let mut bad = false;
    for svc in services {
        match svc.state.as_str() {
            "running" => match svc.health.as_str() {
                "unhealthy" => bad = true,
                "starting" => starting = true,
                _ => {}
            },
            "exited" => {
                if svc.exit_code != 0 {
                    bad = true;
                }
            }
            "created" => starting = true,
            _ => bad = true,
        }
    }
    if bad {
        "degraded"
    } else if starting {
        "deploying"
    } else {
        "running"
    }
}

pub struct ParsedLogLine {
    pub service: String,
    pub timestamp: Option<String>,
    pub message: String,
}

/// Parses `<container>  | <RFC3339 ts> <message>` from `docker compose logs --timestamps`.
pub fn parse_log_line(raw: &str, project: &str) -> Option<ParsedLogLine> {
    let (prefix, rest) = raw.split_once(" | ").or_else(|| raw.split_once("| "))?;
    let service = service_from_prefix(prefix.trim(), project);
    let rest = rest.trim_end();
    let (timestamp, message) = match rest.split_once(' ') {
        Some((first, msg)) if looks_like_timestamp(first) => {
            (Some(first.to_string()), msg.to_string())
        }
        _ if looks_like_timestamp(rest) => (Some(rest.to_string()), String::new()),
        _ => (None, rest.to_string()),
    };
    if message.trim().is_empty() {
        return None;
    }
    Some(ParsedLogLine {
        service,
        timestamp,
        message,
    })
}

fn looks_like_timestamp(value: &str) -> bool {
    value.len() >= 20 && value.as_bytes()[4] == b'-' && value.contains('T')
}

/// `norix-abc-api-1` or `api-1` → `api`.
pub fn service_from_prefix(prefix: &str, project: &str) -> String {
    let mut name = prefix
        .strip_prefix(&format!("{project}-"))
        .unwrap_or(prefix)
        .to_string();
    if let Some((base, index)) = name.rsplit_once('-') {
        if !base.is_empty() && !index.is_empty() && index.chars().all(|c| c.is_ascii_digit()) {
            name = base.to_string();
        }
    }
    name
}

fn tail_lines(text: &str, max: usize) -> Vec<String> {
    let lines: Vec<&str> = text.lines().filter(|l| !l.trim().is_empty()).collect();
    let start = lines.len().saturating_sub(max);
    lines[start..].iter().map(|l| l.to_string()).collect()
}

fn last_line(text: &str) -> String {
    text.lines()
        .rev()
        .find(|l| !l.trim().is_empty())
        .unwrap_or("")
        .trim()
        .chars()
        .take(500)
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn svc(name: &str, state: &str, health: &str, exit_code: i32) -> ObservedComposeService {
        ObservedComposeService {
            name: name.into(),
            state: state.into(),
            health: health.into(),
            exit_code,
            image: String::new(),
            ports: String::new(),
        }
    }

    #[test]
    fn parses_ndjson_and_array_ps_output() {
        let ndjson = r#"{"Service":"web","State":"running","Health":"healthy","ExitCode":0,"Image":"nginx","Publishers":[{"PublishedPort":8080,"TargetPort":80,"Protocol":"tcp"}]}
{"Service":"init","State":"exited","Health":"","ExitCode":0,"Image":"aws-cli","Publishers":[]}"#;
        let parsed = parse_ps_output(ndjson);
        assert_eq!(parsed.len(), 2);
        assert_eq!(parsed[0].name, "init");
        assert_eq!(parsed[1].ports, "8080->80/tcp");
        let array = r#"[{"Service":"db","State":"running","Health":"starting","ExitCode":0}]"#;
        let parsed = parse_ps_output(array);
        assert_eq!(parsed[0].health, "starting");
    }

    #[test]
    fn init_job_exit_zero_is_running_but_crash_is_degraded() {
        let ok = vec![
            svc("api", "running", "healthy", 0),
            svc("init", "exited", "", 0),
        ];
        assert_eq!(derive_status("up", &ok), "running");
        let crash = vec![svc("api", "running", "", 0), svc("init", "exited", "", 1)];
        assert_eq!(derive_status("up", &crash), "degraded");
        let unhealthy = vec![svc("api", "running", "unhealthy", 0)];
        assert_eq!(derive_status("up", &unhealthy), "degraded");
        let starting = vec![svc("db", "running", "starting", 0)];
        assert_eq!(derive_status("up", &starting), "deploying");
        assert_eq!(derive_status("up", &[]), "stopped");
        assert_eq!(
            derive_status("stop", &[svc("api", "exited", "", 0)]),
            "stopped"
        );
    }

    #[test]
    fn parses_log_lines_with_project_prefix_and_timestamp() {
        let line = "norix-abc123-api-1  | 2026-11-18T10:00:00.123456789Z listening on :8080";
        let parsed = parse_log_line(line, "norix-abc123").expect("parsed");
        assert_eq!(parsed.service, "api");
        assert_eq!(
            parsed.timestamp.as_deref(),
            Some("2026-11-18T10:00:00.123456789Z")
        );
        assert_eq!(parsed.message, "listening on :8080");
        let short = parse_log_line(
            "minio-init-1  | 2026-11-18T10:00:01Z bucket created",
            "norix-x",
        )
        .unwrap();
        assert_eq!(short.service, "minio-init");
        assert!(parse_log_line("no separator here", "norix-x").is_none());
    }

    #[test]
    fn override_labels_every_service() {
        let doc = build_override(&["api".into(), "worker".into()], "s1", "p1");
        assert_eq!(doc["services"]["worker"]["labels"]["norix.stack_id"], "s1");
        assert_eq!(doc["services"]["api"]["labels"]["norix.project_id"], "p1");
    }

    #[test]
    fn rejects_compose_paths_outside_checkout() {
        let dir = std::env::temp_dir().join(format!("norix-compose-test-{}", std::process::id()));
        std::fs::create_dir_all(dir.join("deploy")).unwrap();
        std::fs::write(dir.join("deploy/compose.yml"), "services: {}").unwrap();
        assert!(safe_compose_file(&dir, "deploy/compose.yml").is_ok());
        assert!(safe_compose_file(&dir, "../etc/passwd").is_err());
        assert!(safe_compose_file(&dir, "/etc/passwd").is_err());
        assert!(safe_compose_file(&dir, "missing.yml").is_err());
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn validates_stack_identity() {
        let mut stack = DesiredComposeStack {
            id: "3f2504e0-4f89-11d3-9a0c-0305e82c3301".into(),
            compose_project: "norix-3f2504e04f89".into(),
            ..Default::default()
        };
        assert!(validate_stack_identity(&stack).is_ok());
        stack.id = "../etc".into();
        assert!(validate_stack_identity(&stack).is_err());
        stack.id = "3f2504e0".into();
        stack.compose_project = "evil project".into();
        assert!(validate_stack_identity(&stack).is_err());
    }

    #[test]
    fn redacts_tokens_in_git_output() {
        let out = redact_userinfo(
            "fatal: could not read from https://user:gho_secret@github.com/o/r.git",
        );
        assert!(!out.contains("gho_secret"));
        assert!(out.contains("https://***@github.com/o/r.git"));
    }
}
