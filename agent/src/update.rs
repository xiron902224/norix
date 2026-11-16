use std::env;
use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use std::process::Command;

use anyhow::{bail, Context, Result};
use tracing::{info, warn};

use crate::config::Config;

fn normalize_version(raw: &str) -> String {
    raw.trim().trim_start_matches('v').to_string()
}

fn version_tag(raw: &str) -> String {
    let n = normalize_version(raw);
    if n.is_empty() {
        return String::new();
    }
    format!("v{n}")
}

fn detect_arch() -> Result<&'static str> {
    match env::consts::ARCH {
        "x86_64" => Ok("amd64"),
        "aarch64" => Ok("arm64"),
        other => bail!("unsupported architecture for agent update: {other}"),
    }
}

fn current_exe_path() -> Result<PathBuf> {
    env::current_exe().context("resolve current agent binary path")
}

/// Download release binary + SHA256SUMS, replace this process binary, rewrite agent.env, restart.
pub async fn apply_desired_version(config: &Config, desired: &str) -> Result<()> {
    let desired_norm = normalize_version(desired);
    let current_norm = normalize_version(&config.agent_version);
    if desired_norm.is_empty() {
        return Ok(());
    }
    if desired_norm == current_norm {
        info!(version = %desired_norm, "agent already on desired version");
        return Ok(());
    }

    let tag = version_tag(&desired_norm);
    let arch = detect_arch()?;
    let repo = env::var("NORIX_GITHUB_REPO").unwrap_or_else(|_| "xiron902224/norix".into());
    let asset = format!("norix-agent-linux-{arch}");
    let base = format!("https://github.com/{repo}/releases/download/{tag}");
    let bin_url = format!("{base}/{asset}");
    let sums_url = format!("{base}/SHA256SUMS");

    let workdir = &config.workdir;
    let tmp_dir = workdir.join("update-staging");
    fs::create_dir_all(&tmp_dir).context("create update staging dir")?;
    let tmp_bin = tmp_dir.join(&asset);
    let tmp_sums = tmp_dir.join("SHA256SUMS");

    info!(%tag, %bin_url, "downloading agent release");
    download_file(&bin_url, &tmp_bin).await?;
    download_file(&sums_url, &tmp_sums).await?;
    verify_sha256(&tmp_sums, &tmp_bin, &asset)?;

    let dest = current_exe_path()?;
    let dest_tmp = dest.with_extension("new");
    fs::copy(&tmp_bin, &dest_tmp).with_context(|| format!("copy to {}", dest_tmp.display()))?;
    fs::set_permissions(&dest_tmp, fs::Permissions::from_mode(0o755))?;
    fs::rename(&dest_tmp, &dest).with_context(|| format!("replace {}", dest.display()))?;

    patch_agent_env(workdir, &desired_norm)?;
    let _ = fs::remove_dir_all(&tmp_dir);

    info!(from = %current_norm, to = %desired_norm, "agent binary replaced; restarting");
    restart_self(config, &dest)?;
    Ok(())
}

async fn download_file(url: &str, dest: &Path) -> Result<()> {
    let client = reqwest::Client::builder()
        .user_agent(format!("norix-agent-updater/{}", env!("CARGO_PKG_VERSION")))
        .timeout(std::time::Duration::from_secs(120))
        .build()?;
    let bytes = client
        .get(url)
        .send()
        .await
        .with_context(|| format!("GET {url}"))?
        .error_for_status()
        .with_context(|| format!("HTTP error for {url}"))?
        .bytes()
        .await
        .context("read body")?;
    fs::write(dest, &bytes).with_context(|| format!("write {}", dest.display()))?;
    Ok(())
}

fn verify_sha256(sums_path: &Path, bin_path: &Path, asset_name: &str) -> Result<()> {
    let sums = fs::read_to_string(sums_path).context("read SHA256SUMS")?;
    let mut expected: Option<String> = None;
    for line in sums.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        let mut parts = line.split_whitespace();
        let hash = parts.next().unwrap_or("");
        let name = parts.next().unwrap_or("").trim_start_matches('*');
        if name == asset_name || name.ends_with(asset_name) {
            expected = Some(hash.to_lowercase());
            break;
        }
    }
    let Some(expected) = expected else {
        bail!("SHA256SUMS missing entry for {asset_name}");
    };
    let output = Command::new("sha256sum")
        .arg(bin_path)
        .output()
        .context("run sha256sum")?;
    if !output.status.success() {
        bail!("sha256sum failed");
    }
    let stdout = String::from_utf8_lossy(&output.stdout);
    let actual = stdout
        .split_whitespace()
        .next()
        .unwrap_or("")
        .to_lowercase();
    if actual != expected {
        bail!("checksum mismatch for {asset_name}: got {actual}, want {expected}");
    }
    Ok(())
}

fn patch_agent_env(workdir: &Path, version: &str) -> Result<()> {
    let env_path = workdir.join("agent.env");
    if !env_path.exists() {
        warn!(path = %env_path.display(), "agent.env missing; skip NORIX_AGENT_VERSION patch");
        return Ok(());
    }
    let raw = fs::read_to_string(&env_path).context("read agent.env")?;
    let mut lines: Vec<String> = raw
        .lines()
        .filter(|line| !line.starts_with("NORIX_AGENT_VERSION="))
        .map(|line| line.to_string())
        .collect();
    lines.push(format!("NORIX_AGENT_VERSION={version}"));
    let mut out = lines.join("\n");
    out.push('\n');
    fs::write(&env_path, out).context("write agent.env")?;
    Ok(())
}

fn restart_self(config: &Config, bin: &Path) -> Result<()> {
    // Prefer systemd when available; otherwise re-exec under nohup using agent.env.
    let has_systemd = Path::new("/run/systemd/system").exists()
        || fs::read_to_string("/proc/1/comm")
            .unwrap_or_default()
            .trim()
            == "systemd";
    if has_systemd {
        let status = Command::new("systemctl")
            .args(["restart", "norix-agent"])
            .status();
        match status {
            Ok(s) if s.success() => {
                // Current process will be killed by systemd; exit cleanly if not.
                std::process::exit(0);
            }
            Ok(s) => {
                warn!(code = ?s.code(), "systemctl restart norix-agent failed; falling back to nohup")
            }
            Err(error) => warn!(%error, "systemctl unavailable; falling back to nohup"),
        }
    }

    let env_file = config.workdir.join("agent.env");
    let log_file = config.workdir.join("norix-agent.log");
    let pid_file = config.workdir.join("norix-agent.pid");
    let script = format!(
        r#"set -euo pipefail
sleep 1
if [[ -f '{env}' ]]; then set -a; source '{env}'; set +a; fi
nohup '{bin}' >>'{log}' 2>&1 &
echo $! >'{pid}'
"#,
        env = env_file.display(),
        bin = bin.display(),
        log = log_file.display(),
        pid = pid_file.display(),
    );
    Command::new("bash")
        .args(["-c", &script])
        .spawn()
        .context("spawn restarted agent")?;
    std::process::exit(0);
}

pub async fn run_cli_update(version: Option<String>) -> Result<()> {
    let config = Config::from_env()?;
    config.ensure_workdir()?;
    let desired = version
        .or_else(|| env::var("NORIX_VERSION").ok())
        .or_else(|| env::var("NORIX_AGENT_VERSION").ok())
        .unwrap_or_default();
    if desired.trim().is_empty() {
        bail!("usage: norix-agent update --version vX.Y.Z (or set NORIX_VERSION)");
    }
    apply_desired_version(&config, &desired).await
}

pub fn run_cli_uninstall(purge: bool) -> Result<()> {
    let workdir = env::var("NORIX_AGENT_WORKDIR").unwrap_or_else(|_| "/var/lib/norix-agent".into());
    let install_dir =
        env::var("NORIX_AGENT_INSTALL_DIR").unwrap_or_else(|_| "/usr/local/bin".into());
    let mut args = vec![
        "bash".to_string(),
        "-c".to_string(),
        format!(
            r#"curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/main/scripts/uninstall-agent.sh | bash -s -- --workdir '{workdir}' --install-dir '{install_dir}' {purge}"#,
            workdir = workdir,
            install_dir = install_dir,
            purge = if purge { "--purge" } else { "" },
        ),
    ];
    // Prefer local script when developing from a checkout.
    let local = PathBuf::from("scripts/uninstall-agent.sh");
    if local.is_file() {
        args = vec!["bash".into(), local.display().to_string()];
        if purge {
            args.push("--purge".into());
        }
        args.push("--workdir".into());
        args.push(workdir);
        args.push("--install-dir".into());
        args.push(install_dir);
        let status = Command::new(&args[0]).args(&args[1..]).status()?;
        if !status.success() {
            bail!("uninstall-agent.sh failed");
        }
        return Ok(());
    }
    let status = Command::new(&args[0]).args(&args[1..]).status()?;
    if !status.success() {
        bail!("remote uninstall-agent.sh failed");
    }
    Ok(())
}
