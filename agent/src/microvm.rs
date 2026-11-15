use std::fs;
use std::path::Path;
use std::process::Command;

use tracing::{info, warn};

use crate::client::{DesiredWorkload, ObservedWorkload};

/// Reconcile micro-VM desired workloads for v1:
/// - requires KVM (`/dev/kvm`)
/// - writes a boot intent under workdir/microvm/
/// - launches Cloud Hypervisor when `cloud-hypervisor` + kernel/rootfs are available
/// - otherwise reports an honest failed/pending state (no fake "running")
pub async fn reconcile_one(workdir: &Path, desired: &DesiredWorkload) -> ObservedWorkload {
    let dir = workdir.join("microvm").join(sanitize(&desired.id));
    if let Err(error) = fs::create_dir_all(&dir) {
        return failed(desired, error.to_string(), vec![]);
    }

    if desired.action == "stop" {
        let _ = stop_guest(&dir);
        let _ = fs::remove_dir_all(&dir);
        return ObservedWorkload {
            id: desired.id.clone(),
            status: "stopped".into(),
            container_id: String::new(),
            observed_generation: desired.desired_generation,
            message: String::new(),
            host_port: desired.host_port,
            log_lines: vec!["microvm stopped".into()],
        };
    }

    if !Path::new("/dev/kvm").exists() {
        return failed(
            desired,
            "microvm requires /dev/kvm (KVM not available on this host)".into(),
            vec!["kvm_level check failed".into()],
        );
    }

    let kernel = env_or("NORIX_MICROVM_KERNEL", "");
    let rootfs = env_or("NORIX_MICROVM_ROOTFS", "");
    let ch_bin = env_or("NORIX_CLOUD_HYPERVISOR", "cloud-hypervisor");
    let memory_mb = desired.memory_limit_mb.max(128);
    let vcpus = 1u32.max((desired.cpu_shares.max(1024) / 1024) as u32);

    let intent = serde_json::json!({
        "workload_id": desired.id,
        "name": desired.name,
        "image": desired.image,
        "generation": desired.desired_generation,
        "vcpus": vcpus,
        "memory_mb": memory_mb,
        "kernel": kernel,
        "rootfs": rootfs,
        "hypervisor": ch_bin,
        "host_port": desired.host_port,
    });
    let intent_path = dir.join("boot-intent.json");
    if let Err(error) = fs::write(
        &intent_path,
        serde_json::to_string_pretty(&intent).unwrap_or_else(|_| "{}".into()),
    ) {
        return failed(desired, error.to_string(), vec![]);
    }

    if kernel.is_empty() || rootfs.is_empty() || !Path::new(&kernel).exists() || !Path::new(&rootfs).exists() {
        let msg = "microvm boot deferred: set NORIX_MICROVM_KERNEL and NORIX_MICROVM_ROOTFS to launch Cloud Hypervisor";
        info!(workload = %desired.id, "{msg}");
        return ObservedWorkload {
            id: desired.id.clone(),
            status: "pending".into(),
            container_id: String::new(),
            observed_generation: desired.desired_generation,
            message: msg.into(),
            host_port: desired.host_port,
            log_lines: vec![
                format!("wrote {}", intent_path.display()),
                msg.into(),
            ],
        };
    }

    if !command_exists(&ch_bin) {
        return failed(
            desired,
            format!("{ch_bin} not found on PATH; install Cloud Hypervisor for micro-VM boot"),
            vec![format!("wrote {}", intent_path.display())],
        );
    }

    let api_sock = dir.join("api.sock");
    let _ = fs::remove_file(&api_sock);
    let mut args = vec![
        "--api-socket".into(),
        api_sock.display().to_string(),
        "--kernel".into(),
        kernel,
        "--disk".into(),
        format!("path={rootfs}"),
        "--cpus".into(),
        format!("boot={vcpus}"),
        "--memory".into(),
        format!("size={memory_mb}M"),
        "--console".into(),
        "off".into(),
        "--serial".into(),
        "tty".into(),
    ];
    if desired.host_port > 0 {
        args.extend([
            "--net".into(),
            format!(
                "tap=,mac=,ip=,mask=,host_mac=,num_queues=1,queue_size=256"
            ),
        ]);
    }

    let log_path = dir.join("ch.log");
    let log_file = match fs::File::create(&log_path) {
        Ok(file) => file,
        Err(error) => return failed(desired, error.to_string(), vec![]),
    };
    match Command::new(&ch_bin)
        .args(&args)
        .stdout(log_file.try_clone().unwrap_or_else(|_| fs::File::create("/dev/null").unwrap()))
        .stderr(log_file)
        .spawn()
    {
        Ok(child) => {
            let pid = child.id();
            let _ = fs::write(dir.join("pid"), pid.to_string());
            info!(workload = %desired.id, pid, "cloud-hypervisor started");
            ObservedWorkload {
                id: desired.id.clone(),
                status: "running".into(),
                container_id: format!("microvm:{pid}"),
                observed_generation: desired.desired_generation,
                message: String::new(),
                host_port: desired.host_port,
                log_lines: vec![
                    format!("wrote {}", intent_path.display()),
                    format!("cloud-hypervisor pid={pid}"),
                ],
            }
        }
        Err(error) => {
            warn!(%error, "cloud-hypervisor spawn failed");
            failed(
                desired,
                format!("cloud-hypervisor spawn failed: {error}"),
                vec![format!("wrote {}", intent_path.display())],
            )
        }
    }
}

fn stop_guest(dir: &Path) {
    if let Ok(raw) = fs::read_to_string(dir.join("pid")) {
        if let Ok(pid) = raw.trim().parse::<i32>() {
            let _ = Command::new("kill").args(["-TERM", &pid.to_string()]).status();
        }
    }
}

fn failed(desired: &DesiredWorkload, message: String, mut logs: Vec<String>) -> ObservedWorkload {
    logs.push(message.clone());
    ObservedWorkload {
        id: desired.id.clone(),
        status: "failed".into(),
        container_id: String::new(),
        observed_generation: desired.desired_generation,
        message,
        host_port: desired.host_port,
        log_lines: logs,
    }
}

fn sanitize(value: &str) -> String {
    value
        .chars()
        .filter(|ch| ch.is_ascii_hexdigit() || *ch == '-')
        .take(36)
        .collect()
}

fn env_or(key: &str, default: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| default.to_string())
}

fn command_exists(bin: &str) -> bool {
    Command::new(bin)
        .arg("--version")
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null())
        .status()
        .map(|status| status.success())
        .unwrap_or(false)
}
