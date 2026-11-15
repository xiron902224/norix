use std::fs;
use std::path::Path;

use serde::Serialize;

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Capabilities {
    pub cpu_cores: i32,
    pub memory_total_mb: i64,
    pub docker_socket: bool,
    pub containerd_socket: bool,
    pub kvm_level: u32,
    pub hostname: String,
    pub agent_version: String,
}

pub fn detect(agent_version: &str) -> Capabilities {
    Capabilities {
        cpu_cores: detect_cpu_cores(),
        memory_total_mb: detect_memory_total_mb(),
        docker_socket: Path::new("/var/run/docker.sock").exists()
            || Path::new("/run/docker.sock").exists(),
        containerd_socket: Path::new("/run/containerd/containerd.sock").exists()
            || Path::new("/var/run/containerd/containerd.sock").exists(),
        kvm_level: detect_kvm_level(),
        hostname: hostname::get()
            .ok()
            .and_then(|value| value.into_string().ok())
            .unwrap_or_else(|| "unknown".into()),
        agent_version: agent_version.to_string(),
    }
}

fn detect_cpu_cores() -> i32 {
    std::thread::available_parallelism()
        .map(|value| value.get() as i32)
        .unwrap_or(1)
}

fn detect_memory_total_mb() -> i64 {
    let Ok(contents) = fs::read_to_string("/proc/meminfo") else {
        return 0;
    };
    for line in contents.lines() {
        if let Some(rest) = line.strip_prefix("MemTotal:") {
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

/// KVM detection only (no boot): 0 unavailable, 1 /dev/kvm present, 2 nested virtualization.
fn detect_kvm_level() -> u32 {
    if !Path::new("/dev/kvm").exists() {
        return 0;
    }
    for path in [
        "/sys/module/kvm_intel/parameters/nested",
        "/sys/module/kvm_amd/parameters/nested",
    ] {
        if let Ok(value) = fs::read_to_string(path) {
            let trimmed = value.trim();
            if trimmed == "1" || trimmed.eq_ignore_ascii_case("Y") {
                return 2;
            }
        }
    }
    1
}
