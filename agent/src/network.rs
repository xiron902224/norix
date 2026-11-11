use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

use tracing::{info, warn};

use crate::client::{DesiredNetworkPlan, ObservedNetwork};

pub async fn apply_network(plan: &DesiredNetworkPlan, workdir: &Path) -> ObservedNetwork {
    let mut logs = vec![format!(
        "network generation {} address={} peers={} dns={} apply={}",
        plan.generation,
        plan.address,
        plan.peers.len(),
        plan.dns_records.len(),
        plan.apply
    )];
    let dir = workdir.join("network");
    if let Err(error) = fs::create_dir_all(&dir) {
        return fail(plan, logs, error.to_string());
    }
    let wg_path = dir.join("wg-norix.conf");
    let nft_path = dir.join("norix.nft");
    let dns_path = dir.join("dns-hosts");
    let cert_dir = dir.join("certs");
    let _ = fs::create_dir_all(&cert_dir);

    if let Err(error) = fs::write(&wg_path, &plan.wg_config) {
        return fail(plan, logs, error.to_string());
    }
    logs.push(format!("wrote {}", wg_path.display()));
    if let Err(error) = fs::write(&nft_path, &plan.nftables) {
        return fail(plan, logs, error.to_string());
    }
    logs.push(format!("wrote {}", nft_path.display()));
    let hosts = render_hosts(&plan.dns_records);
    if let Err(error) = fs::write(&dns_path, hosts) {
        return fail(plan, logs, error.to_string());
    }
    logs.push(format!(
        "wrote {} ({} records)",
        dns_path.display(),
        plan.dns_records.len()
    ));
    for cert in &plan.certificates {
        let name = sanitize_file(&cert.hostname);
        if let Err(error) = fs::write(cert_dir.join(format!("{name}.crt")), &cert.cert_pem) {
            return fail(plan, logs, error.to_string());
        }
        if let Err(error) = fs::write(cert_dir.join(format!("{name}.key")), &cert.key_pem) {
            return fail(plan, logs, error.to_string());
        }
        logs.push(format!("wrote cert {}", cert.hostname));
    }

    if !plan.apply {
        logs.push("dry-run: set NORIX_APPLY_WIREGUARD=1 on control plane to enable apply".into());
        return ObservedNetwork {
            generation: plan.generation,
            status: "ready".into(),
            message: "configs written (apply disabled)".into(),
            log_lines: logs,
        };
    }

    if let Err(error) = run_wg_quick(&wg_path) {
        logs.push(format!("wg-quick: {error}"));
        return ObservedNetwork {
            generation: plan.generation,
            status: "degraded".into(),
            message: error,
            log_lines: logs,
        };
    }
    logs.push("wg-quick applied".into());
    if let Err(error) = run_nft(&nft_path) {
        logs.push(format!("nft: {error}"));
        return ObservedNetwork {
            generation: plan.generation,
            status: "degraded".into(),
            message: error,
            log_lines: logs,
        };
    }
    logs.push("nftables applied".into());
    info!(generation = plan.generation, "network mesh applied");
    ObservedNetwork {
        generation: plan.generation,
        status: "ready".into(),
        message: "mesh applied".into(),
        log_lines: logs,
    }
}

fn fail(plan: &DesiredNetworkPlan, mut logs: Vec<String>, message: String) -> ObservedNetwork {
    warn!(%message, "network apply failed");
    logs.push(message.clone());
    ObservedNetwork {
        generation: plan.generation,
        status: "error".into(),
        message,
        log_lines: logs,
    }
}

fn render_hosts(records: &[crate::client::DesiredNetworkDNSRecord]) -> String {
    let mut out = String::from("# norix internal DNS snapshot\n");
    for record in records {
        if record.r#type == "A" || record.r#type == "AAAA" {
            out.push_str(&format!("{} {}\n", record.value, record.name));
        }
    }
    out
}

fn sanitize_file(value: &str) -> String {
    let mut out = String::new();
    for ch in value.to_lowercase().chars() {
        match ch {
            'a'..='z' | '0'..='9' | '-' | '.' => out.push(ch),
            '*' => out.push_str("wildcard"),
            _ => out.push('_'),
        }
    }
    if out.is_empty() {
        "cert".into()
    } else {
        out
    }
}

fn run_wg_quick(conf: &PathBuf) -> Result<(), String> {
    let _ = Command::new("wg-quick").arg("down").arg(conf).output();
    let output = Command::new("wg-quick")
        .arg("up")
        .arg(conf)
        .output()
        .map_err(|error| format!("wg-quick missing: {error}"))?;
    if !output.status.success() {
        return Err(String::from_utf8_lossy(&output.stderr).trim().to_string());
    }
    Ok(())
}

fn run_nft(path: &PathBuf) -> Result<(), String> {
    let output = Command::new("nft")
        .arg("-f")
        .arg(path)
        .output()
        .map_err(|error| format!("nft missing: {error}"))?;
    if !output.status.success() {
        return Err(String::from_utf8_lossy(&output.stderr).trim().to_string());
    }
    Ok(())
}
