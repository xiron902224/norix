//! `norix doctor`: read-only checks for a host that will run Norix (#34).
//!
//! Every check returns a level. When the level is not `pass`, it also returns
//! a fix the operator can act on. Checks never change the host. The same
//! checks run in `scripts/install.sh preflight`.

use std::net::{TcpListener, ToSocketAddrs};
use std::process::Command;

use anyhow::{bail, Result};
use serde::Serialize;
use serde_json::json;

use crate::output::OutputFormat;

/// Free space below this is a warning. Images and the build cache need room.
pub const MIN_DISK_GIB: u64 = 10;
/// Available memory below this is a warning.
pub const MIN_RAM_MIB: u64 = 2048;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Level {
    Pass,
    Warn,
    Fail,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct Check {
    pub name: String,
    pub level: Level,
    pub detail: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub fix: Option<String>,
}

impl Check {
    fn pass(name: &str, detail: impl Into<String>) -> Self {
        Self {
            name: name.into(),
            level: Level::Pass,
            detail: detail.into(),
            fix: None,
        }
    }

    fn warn(name: &str, detail: impl Into<String>, fix: &str) -> Self {
        Self {
            name: name.into(),
            level: Level::Warn,
            detail: detail.into(),
            fix: Some(fix.into()),
        }
    }

    fn fail(name: &str, detail: impl Into<String>, fix: &str) -> Self {
        Self {
            name: name.into(),
            level: Level::Fail,
            detail: detail.into(),
            fix: Some(fix.into()),
        }
    }
}

/// Parses `MemAvailable` (in kB) from `/proc/meminfo`.
pub fn parse_mem_available_kib(meminfo: &str) -> Option<u64> {
    meminfo.lines().find_map(|line| {
        let rest = line.strip_prefix("MemAvailable:")?;
        rest.split_whitespace().next()?.parse().ok()
    })
}

/// Parses the available kB from `df -Pk <path>` (POSIX output: header, then one row).
pub fn parse_df_available_kib(output: &str) -> Option<u64> {
    let row = output.lines().nth(1)?;
    row.split_whitespace().nth(3)?.parse().ok()
}

/// Returns the major version from `docker compose version --short` (for example `v2.29.1`).
pub fn compose_major(version: &str) -> Option<u64> {
    version
        .trim()
        .trim_start_matches('v')
        .split('.')
        .next()?
        .parse()
        .ok()
}

/// True when `/proc/filesystems` lists the `overlay` filesystem Docker uses by default.
pub fn filesystems_has_overlay(raw: &str) -> bool {
    raw.lines()
        .any(|line| line.split_whitespace().last() == Some("overlay"))
}

fn run_capture(program: &str, args: &[&str]) -> Option<String> {
    let out = Command::new(program).args(args).output().ok()?;
    out.status
        .success()
        .then(|| String::from_utf8_lossy(&out.stdout).trim().to_string())
}

/// A port is usable when this process can bind it on all interfaces.
pub fn check_port(port: u16) -> Check {
    let name = format!("port {port}");
    match TcpListener::bind(("0.0.0.0", port)) {
        Ok(_) => Check::pass(&name, "free"),
        Err(error) if error.kind() == std::io::ErrorKind::AddrInUse => Check::fail(
            &name,
            "already in use",
            "stop the process that owns it (ss -ltnp) or choose other ports with --ports",
        ),
        Err(error) => Check::warn(
            &name,
            format!("could not bind: {error}"),
            "run doctor with sudo to test privileged ports (below 1024), or check firewall rules",
        ),
    }
}

pub fn check_docker() -> Check {
    if run_capture("docker", &["--version"]).is_none() {
        return Check::fail(
            "docker",
            "docker CLI not found",
            "install Docker Engine: https://docs.docker.com/engine/install/",
        );
    }
    match run_capture("docker", &["info", "--format", "{{.ServerVersion}}"]) {
        Some(version) if !version.is_empty() => Check::pass("docker", format!("engine {version}")),
        _ => Check::fail(
            "docker",
            "daemon not reachable for this user",
            "start the docker service, or add your user to the docker group and log in again",
        ),
    }
}

pub fn check_compose() -> Check {
    let major = run_capture("docker", &["compose", "version", "--short"])
        .as_deref()
        .and_then(compose_major);
    match major {
        Some(major) if major >= 2 => Check::pass("docker compose", format!("v{major}.x")),
        Some(major) => Check::fail(
            "docker compose",
            format!("v{major}.x found, v2 required"),
            "install the docker-compose-plugin package",
        ),
        None => Check::fail(
            "docker compose",
            "not available",
            "install the docker-compose-plugin package",
        ),
    }
}

pub fn check_disk(path: &str) -> Check {
    let available = run_capture("df", &["-Pk", path])
        .as_deref()
        .and_then(parse_df_available_kib);
    match available {
        Some(kib) => {
            let gib = kib / 1024 / 1024;
            if gib >= MIN_DISK_GIB {
                Check::pass("disk", format!("{gib} GiB free at {path}"))
            } else {
                Check::warn(
                    "disk",
                    format!("{gib} GiB free at {path}, {MIN_DISK_GIB} GiB recommended"),
                    "free disk space; images and build cache need room",
                )
            }
        }
        None => Check::warn(
            "disk",
            format!("could not read free space at {path}"),
            "run df -h manually and confirm there is room",
        ),
    }
}

pub fn check_memory() -> Check {
    let available = std::fs::read_to_string("/proc/meminfo")
        .ok()
        .as_deref()
        .and_then(parse_mem_available_kib);
    match available {
        Some(kib) => {
            let mib = kib / 1024;
            if mib >= MIN_RAM_MIB {
                Check::pass("memory", format!("{mib} MiB available"))
            } else {
                Check::warn(
                    "memory",
                    format!("{mib} MiB available, {MIN_RAM_MIB} MiB recommended"),
                    "stop other workloads or add RAM before running builds",
                )
            }
        }
        None => Check::warn(
            "memory",
            "not checked (no /proc/meminfo on this OS)",
            "check memory manually; Norix recommends 2 GiB available",
        ),
    }
}

pub fn check_kvm() -> Check {
    match std::fs::OpenOptions::new().read(true).write(true).open("/dev/kvm") {
        Ok(_) => Check::pass("kvm", "/dev/kvm is accessible"),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Check::warn(
            "kvm",
            "/dev/kvm not found (optional)",
            "needed only for the micro-VM runtime: enable virtualization in the BIOS and load the kvm module",
        ),
        Err(_) => Check::warn(
            "kvm",
            "/dev/kvm exists but is not accessible (optional)",
            "add your user to the kvm group if you need the micro-VM runtime",
        ),
    }
}

pub fn check_kernel_overlay() -> Check {
    match std::fs::read_to_string("/proc/filesystems") {
        Ok(raw) if filesystems_has_overlay(&raw) => {
            Check::pass("kernel overlay", "overlay filesystem available")
        }
        Ok(_) => Check::fail(
            "kernel overlay",
            "overlay filesystem not available",
            "load it with sudo modprobe overlay (Docker's default storage driver)",
        ),
        Err(_) => Check::warn(
            "kernel overlay",
            "not checked (no /proc/filesystems on this OS)",
            "confirm the overlay filesystem is available",
        ),
    }
}

pub fn check_kernel_bridge_netfilter() -> Check {
    if std::path::Path::new("/proc/sys/net/bridge/bridge-nf-call-iptables").exists() {
        Check::pass("kernel br_netfilter", "bridge netfilter loaded")
    } else {
        Check::warn(
            "kernel br_netfilter",
            "bridge netfilter not loaded",
            "load it with sudo modprobe br_netfilter; Docker usually does this on start",
        )
    }
}

/// Checks that the domain resolves. This is a warning: a local install does not need DNS yet.
pub fn check_dns(domain: &str) -> Check {
    let name = format!("dns {domain}");
    match (domain, 443).to_socket_addrs() {
        Ok(addrs) => {
            let ips: Vec<String> = addrs.map(|addr| addr.ip().to_string()).collect();
            if ips.is_empty() {
                Check::warn(
                    &name,
                    "no address records",
                    "create an A/AAAA record that points at this host",
                )
            } else {
                Check::pass(&name, format!("resolves to {}", ips.join(", ")))
            }
        }
        Err(_) => Check::warn(
            &name,
            "does not resolve yet",
            "create an A/AAAA record that points at this host (needed for HTTPS)",
        ),
    }
}

pub struct Options {
    pub ports: Vec<u16>,
    pub domain: Option<String>,
    pub disk_path: String,
}

pub fn run_checks(options: &Options) -> Vec<Check> {
    let mut checks = vec![
        check_docker(),
        check_compose(),
        check_kernel_overlay(),
        check_kernel_bridge_netfilter(),
        check_disk(&options.disk_path),
        check_memory(),
        check_kvm(),
    ];
    checks.extend(options.ports.iter().map(|port| check_port(*port)));
    if let Some(domain) = &options.domain {
        checks.push(check_dns(domain));
    }
    checks
}

pub fn has_failures(checks: &[Check]) -> bool {
    checks.iter().any(|check| check.level == Level::Fail)
}

fn count(checks: &[Check], level: Level) -> usize {
    checks.iter().filter(|check| check.level == level).count()
}

pub fn print_checks(format: OutputFormat, checks: &[Check]) -> Result<()> {
    match format {
        OutputFormat::Json => {
            let value = json!({
                "checks": checks,
                "passed": count(checks, Level::Pass),
                "warnings": count(checks, Level::Warn),
                "failed": count(checks, Level::Fail),
            });
            println!("{}", serde_json::to_string_pretty(&value)?);
        }
        OutputFormat::Text => {
            for check in checks {
                let tag = match check.level {
                    Level::Pass => "PASS",
                    Level::Warn => "WARN",
                    Level::Fail => "FAIL",
                };
                println!("[{tag}] {}: {}", check.name, check.detail);
                if let Some(fix) = &check.fix {
                    println!("       fix: {fix}");
                }
            }
            println!(
                "\n{} passed, {} warnings, {} failed",
                count(checks, Level::Pass),
                count(checks, Level::Warn),
                count(checks, Level::Fail),
            );
        }
    }
    Ok(())
}

/// Prints the report, then fails the command when any check failed.
pub fn run(format: OutputFormat, options: &Options) -> Result<()> {
    let checks = run_checks(options);
    print_checks(format, &checks)?;
    if has_failures(&checks) {
        bail!("doctor found failing checks");
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_mem_available() {
        let raw =
            "MemTotal:       16000000 kB\nMemFree:         100000 kB\nMemAvailable:   4096000 kB\n";
        assert_eq!(parse_mem_available_kib(raw), Some(4_096_000));
        assert_eq!(parse_mem_available_kib("MemTotal: 1 kB"), None);
    }

    #[test]
    fn parses_df_available() {
        let raw = "Filesystem     1024-blocks      Used Available Capacity Mounted on\n/dev/sda1       103081248  50000000  53081248      49% /\n";
        assert_eq!(parse_df_available_kib(raw), Some(53_081_248));
        assert_eq!(parse_df_available_kib("Filesystem\n"), None);
    }

    #[test]
    fn parses_compose_major() {
        assert_eq!(compose_major("v2.29.1\n"), Some(2));
        assert_eq!(compose_major("5.5.0"), Some(5));
        assert_eq!(compose_major("not a version"), None);
    }

    #[test]
    fn detects_overlay_filesystem() {
        assert!(filesystems_has_overlay(
            "nodev\tsysfs\n\toverlay\nnodev\ttmpfs\n"
        ));
        assert!(!filesystems_has_overlay("nodev\tsysfs\n\text4\n"));
    }

    #[test]
    fn busy_port_fails_with_fix() {
        let listener = TcpListener::bind(("0.0.0.0", 0)).expect("bind ephemeral port");
        let port = listener.local_addr().expect("local addr").port();
        let check = check_port(port);
        assert_eq!(check.level, Level::Fail);
        assert!(check.fix.is_some());
        drop(listener);
    }

    #[test]
    fn failures_drive_exit_status() {
        let pass = Check::pass("a", "ok");
        let warn = Check::warn("b", "meh", "fix");
        let fail = Check::fail("c", "bad", "fix");
        assert!(!has_failures(&[pass.clone(), warn.clone()]));
        assert!(has_failures(&[pass, warn, fail]));
    }
}
