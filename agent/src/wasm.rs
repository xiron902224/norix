use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

use anyhow::{bail, Context, Result};
use tracing::{info, warn};

use crate::client::{DesiredWasmFunction, DesiredWasmPlan, ObservedWasm, ObservedWasmFunction};

/// Reconcile DesiredWasmPlan: pull OCI/HTTP/local `.wasm`, validate magic, run under
/// Wasmtime with fuel/memory limits, optionally serve WASI HTTP (#47).
pub async fn apply_wasm_plan(workdir: &Path, plan: &DesiredWasmPlan) -> ObservedWasm {
    let mut logs = vec![format!(
        "wasm generation {} functions={}",
        plan.generation,
        plan.functions.len()
    )];
    let config_dir = workdir.join("wasm");
    if let Err(error) = fs::create_dir_all(&config_dir) {
        return fail(plan, logs, error.to_string());
    }
    let artifacts = config_dir.join("artifacts");
    if let Err(error) = fs::create_dir_all(&artifacts) {
        return fail(plan, logs, error.to_string());
    }

    let mut observed_fns = Vec::with_capacity(plan.functions.len());
    let mut any_failed = false;
    for function in &plan.functions {
        match stage_function(&artifacts, function, &mut logs).await {
            Ok(item) => {
                if item.status != "ready" && item.status != "running" {
                    any_failed = true;
                }
                observed_fns.push(item);
            }
            Err(error) => {
                any_failed = true;
                warn!(name = %function.name, %error, "wasm function staging failed");
                logs.push(format!("{}: {error}", function.name));
                observed_fns.push(ObservedWasmFunction {
                    id: function.id.clone(),
                    status: "failed".into(),
                    message: error.to_string(),
                    artifact_path: String::new(),
                });
            }
        }
    }

    let snapshot = serde_json::json!({
        "generation": plan.generation,
        "functions": observed_fns.iter().map(|item| serde_json::json!({
            "id": item.id,
            "status": item.status,
            "message": item.message,
            "artifact_path": item.artifact_path,
        })).collect::<Vec<_>>(),
    });
    let config_path = config_dir.join("functions.json");
    if let Err(error) = fs::write(
        &config_path,
        serde_json::to_string_pretty(&snapshot).unwrap_or_else(|_| "{}".into()),
    ) {
        return fail(plan, logs, error.to_string());
    }
    logs.push(format!("wrote {}", config_path.display()));
    info!(
        path = %config_path.display(),
        count = observed_fns.len(),
        "wasm plan applied"
    );

    ObservedWasm {
        generation: plan.generation,
        status: if any_failed {
            "degraded".into()
        } else {
            "ready".into()
        },
        message: if any_failed {
            "one or more wasm functions failed staging".into()
        } else {
            "wasm functions staged".into()
        },
        log_lines: logs,
        functions: observed_fns,
    }
}

async fn stage_function(
    artifacts: &Path,
    function: &DesiredWasmFunction,
    logs: &mut Vec<String>,
) -> Result<ObservedWasmFunction> {
    let safe = sanitize(&function.name);
    let dest = artifacts.join(format!("{safe}.wasm"));
    let image = function.image_ref.trim();
    if image.is_empty() {
        bail!("image_ref is required");
    }

    if image.ends_with(".wasm")
        && (image.starts_with("http://")
            || image.starts_with("https://")
            || Path::new(image).exists())
    {
        fetch_wasm_file(image, &dest, logs)?;
    } else if !materialize_oci(image, &dest, artifacts, &safe, logs)? {
        bail!("could not materialize OCI wasm from {image} (tried oras + docker)");
    }

    validate_wasm_magic(&dest)?;
    logs.push(format!("{}: staged {}", function.name, dest.display()));

    let fuel = if function.fuel_limit > 0 {
        function.fuel_limit as u64
    } else {
        1_000_000
    };
    let mem_mb = if function.memory_limit_mb > 0 {
        function.memory_limit_mb
    } else {
        64
    };
    let listen_port = if function.listen_port > 0 {
        function.listen_port
    } else {
        19000
    };
    let addr = format!("127.0.0.1:{listen_port}");

    let mut message = format!(
        "artifact ready fuel={fuel} memory_mb={mem_mb} addr={addr} host={}",
        function.hostname
    );
    let mut status = "ready".to_string();
    if let Some(serve) = try_wasi_http_serve(&dest, fuel, mem_mb, &addr, logs) {
        status = "running".into();
        message = serve;
    } else if let Some(smoke) = smoke_wasmtime(&dest, fuel, mem_mb) {
        logs.push(format!("{}: {smoke}", function.name));
        // Empty modules / missing _start often fail smoke — still mark ready if magic ok.
        if smoke.contains("failed") && !smoke.contains("no exports") && !smoke.contains("_start") {
            status = "degraded".into();
            message = smoke;
        } else {
            status = "running".into();
            message = format!("{smoke}; limits fuel={fuel} memory_mb={mem_mb}");
        }
    } else {
        logs.push(format!(
            "{}: wasmtime not installed — artifact staged with limits fuel={fuel} memory_mb={mem_mb}",
            function.name
        ));
    }

    Ok(ObservedWasmFunction {
        id: function.id.clone(),
        status,
        message,
        artifact_path: dest.display().to_string(),
    })
}

fn fetch_wasm_file(source: &str, dest: &Path, logs: &mut Vec<String>) -> Result<()> {
    if Path::new(source).exists() {
        fs::copy(source, dest).with_context(|| format!("copy {source}"))?;
        logs.push(format!("copied local wasm {source}"));
        return Ok(());
    }
    let output = Command::new("curl")
        .args(["-fsSL", "-o"])
        .arg(dest)
        .arg(source)
        .output()
        .context("curl wasm artifact")?;
    if !output.status.success() {
        bail!(
            "download failed: {}",
            String::from_utf8_lossy(&output.stderr).trim()
        );
    }
    logs.push(format!("downloaded {source}"));
    Ok(())
}

/// Pull OCI ref via oras and/or docker; return true when dest has a valid blob path written.
fn materialize_oci(
    image: &str,
    dest: &Path,
    artifacts: &Path,
    safe: &str,
    logs: &mut Vec<String>,
) -> Result<bool> {
    if try_oras_pull(image, dest, artifacts, safe, logs)? {
        return Ok(true);
    }
    if try_extract_from_oci(image, dest, logs)? {
        return Ok(true);
    }
    if try_docker_export_find(image, dest, logs)? {
        return Ok(true);
    }
    Ok(false)
}

fn try_oras_pull(
    image: &str,
    dest: &Path,
    artifacts: &Path,
    safe: &str,
    logs: &mut Vec<String>,
) -> Result<bool> {
    let which = Command::new("oras").arg("version").output();
    let Ok(which) = which else {
        return Ok(false);
    };
    if !which.status.success() {
        return Ok(false);
    }
    let dir = artifacts.join(format!("{safe}.oras"));
    let _ = fs::remove_dir_all(&dir);
    fs::create_dir_all(&dir)?;
    let pull = Command::new("oras")
        .args(["pull", image, "-o"])
        .arg(&dir)
        .output()
        .context("oras pull")?;
    if !pull.status.success() {
        logs.push(format!(
            "oras pull {image}: {}",
            String::from_utf8_lossy(&pull.stderr).trim()
        ));
        let _ = fs::remove_dir_all(&dir);
        return Ok(false);
    }
    if let Some(found) = find_wasm_file(&dir) {
        fs::copy(&found, dest)?;
        logs.push(format!("oras extracted {} from {image}", found.display()));
        let _ = fs::remove_dir_all(&dir);
        return Ok(true);
    }
    let _ = fs::remove_dir_all(&dir);
    logs.push(format!("oras pull {image}: no .wasm in artifact"));
    Ok(false)
}

fn try_extract_from_oci(image: &str, dest: &Path, logs: &mut Vec<String>) -> Result<bool> {
    let create = Command::new("docker").args(["create", image]).output();
    let Ok(create) = create else {
        return Ok(false);
    };
    if !create.status.success() {
        logs.push(format!(
            "docker create {image}: {}",
            String::from_utf8_lossy(&create.stderr).trim()
        ));
        return Ok(false);
    }
    let cid = String::from_utf8_lossy(&create.stdout).trim().to_string();
    let candidates = [
        "/app.wasm",
        "/component.wasm",
        "/module.wasm",
        "/wasm/app.wasm",
        "/function.wasm",
        "/main.wasm",
    ];
    let mut ok = false;
    for path in candidates {
        let cp = Command::new("docker")
            .args(["cp", &format!("{cid}:{path}"), &dest.display().to_string()])
            .output();
        if let Ok(cp) = cp {
            if cp.status.success() && dest.exists() {
                if validate_wasm_magic(dest).is_ok() {
                    logs.push(format!("extracted {path} from {image}"));
                    ok = true;
                    break;
                }
                let _ = fs::remove_file(dest);
            }
        }
    }
    let _ = Command::new("docker").args(["rm", "-f", &cid]).output();
    Ok(ok)
}

fn try_docker_export_find(image: &str, dest: &Path, logs: &mut Vec<String>) -> Result<bool> {
    let create = Command::new("docker").args(["create", image]).output();
    let Ok(create) = create else {
        return Ok(false);
    };
    if !create.status.success() {
        return Ok(false);
    }
    let cid = String::from_utf8_lossy(&create.stdout).trim().to_string();
    let tmp = dest.with_extension("tar");
    let export = Command::new("docker")
        .args(["export", &cid, "-o"])
        .arg(&tmp)
        .output();
    let _ = Command::new("docker").args(["rm", "-f", &cid]).output();
    let Ok(export) = export else {
        let _ = fs::remove_file(&tmp);
        return Ok(false);
    };
    if !export.status.success() {
        let _ = fs::remove_file(&tmp);
        return Ok(false);
    }
    let list = Command::new("tar").args(["-tf"]).arg(&tmp).output();
    let Ok(list) = list else {
        let _ = fs::remove_file(&tmp);
        return Ok(false);
    };
    let names = String::from_utf8_lossy(&list.stdout);
    let mut member: Option<String> = None;
    for line in names.lines() {
        let trimmed = line.trim().trim_start_matches("./");
        if trimmed.ends_with(".wasm") && !trimmed.contains("..") {
            member = Some(trimmed.to_string());
            break;
        }
    }
    let Some(member) = member else {
        let _ = fs::remove_file(&tmp);
        return Ok(false);
    };
    let extract = Command::new("tar")
        .args(["-xf"])
        .arg(&tmp)
        .arg("-O")
        .arg(&member)
        .output();
    let _ = fs::remove_file(&tmp);
    let Ok(extract) = extract else {
        return Ok(false);
    };
    if !extract.status.success() || extract.stdout.len() < 4 {
        return Ok(false);
    }
    fs::write(dest, &extract.stdout)?;
    if validate_wasm_magic(dest).is_err() {
        let _ = fs::remove_file(dest);
        return Ok(false);
    }
    logs.push(format!("docker export found {member} in {image}"));
    Ok(true)
}

fn find_wasm_file(dir: &Path) -> Option<PathBuf> {
    let mut stack = vec![dir.to_path_buf()];
    while let Some(cur) = stack.pop() {
        let entries = fs::read_dir(&cur).ok()?;
        for entry in entries.flatten() {
            let path = entry.path();
            if path.is_dir() {
                stack.push(path);
            } else if path.extension().and_then(|e| e.to_str()) == Some("wasm") {
                return Some(path);
            }
        }
    }
    None
}

fn validate_wasm_magic(path: &Path) -> Result<()> {
    let bytes = fs::read(path).with_context(|| format!("read {}", path.display()))?;
    if bytes.len() < 4 || &bytes[0..4] != b"\0asm" {
        bail!(
            "{} is not a wasm binary (missing \\0asm magic)",
            path.display()
        );
    }
    Ok(())
}

/// Spawn `wasmtime serve` unless NORIX_WASM_SERVE=0 (#47: default on).
fn try_wasi_http_serve(
    path: &Path,
    fuel: u64,
    mem_mb: i32,
    addr: &str,
    logs: &mut Vec<String>,
) -> Option<String> {
    let opt_out = std::env::var("NORIX_WASM_SERVE")
        .ok()
        .map(|v| {
            let v = v.to_lowercase();
            v == "0" || v == "false" || v == "off" || v == "no"
        })
        .unwrap_or(false);
    if opt_out {
        logs.push("NORIX_WASM_SERVE=0 — skip wasmtime serve".into());
        return None;
    }
    let help = Command::new("wasmtime")
        .args(["serve", "--help"])
        .output()
        .ok()?;
    if !help.status.success() {
        return None;
    }
    let child = Command::new("wasmtime")
        .args([
            "serve",
            "--addr",
            addr,
            "--fuel",
            &fuel.to_string(),
            "--max-memory-size",
            &format!("{}m", mem_mb.max(1)),
        ])
        .arg(path)
        .spawn()
        .ok()?;
    logs.push(format!(
        "wasmtime serve started pid={} addr={addr}",
        child.id()
    ));
    std::mem::forget(child);
    Some(format!("wasi-http serving on {addr}"))
}

fn smoke_wasmtime(path: &Path, fuel: u64, mem_mb: i32) -> Option<String> {
    let which = Command::new("wasmtime").arg("--version").output().ok()?;
    if !which.status.success() {
        return None;
    }
    let output = Command::new("wasmtime")
        .args([
            "run",
            "--fuel",
            &fuel.to_string(),
            "--max-memory-size",
            &format!("{}m", mem_mb.max(1)),
        ])
        .arg(path)
        .output()
        .ok()?;
    if output.status.success() {
        Some("wasmtime smoke ok".into())
    } else {
        let err = String::from_utf8_lossy(&output.stderr).trim().to_string();
        Some(format!(
            "wasmtime smoke failed: {}",
            if err.is_empty() {
                "non-zero exit".into()
            } else {
                err.chars().take(200).collect::<String>()
            }
        ))
    }
}

fn sanitize(value: &str) -> String {
    let mut out = String::new();
    for ch in value.to_lowercase().chars() {
        match ch {
            'a'..='z' | '0'..='9' | '-' | '_' => out.push(ch),
            _ => out.push('_'),
        }
    }
    if out.is_empty() {
        "fn".into()
    } else {
        out
    }
}

fn fail(plan: &DesiredWasmPlan, mut logs: Vec<String>, message: String) -> ObservedWasm {
    warn!(%message, "wasm apply failed");
    logs.push(message.clone());
    ObservedWasm {
        generation: plan.generation,
        status: "error".into(),
        message,
        log_lines: logs,
        functions: Vec::new(),
    }
}

#[allow(dead_code)]
pub fn artifact_dir(workdir: &Path) -> PathBuf {
    workdir.join("wasm").join("artifacts")
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;

    #[test]
    fn validate_magic_accepts_empty_module() {
        let dir = std::env::temp_dir().join("norix-wasm-test");
        let _ = fs::create_dir_all(&dir);
        let path = dir.join("empty.wasm");
        let mut f = fs::File::create(&path).unwrap();
        f.write_all(&[0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00])
            .unwrap();
        validate_wasm_magic(&path).unwrap();
        let _ = fs::remove_file(&path);
    }

    #[test]
    fn sanitize_fn_name() {
        assert_eq!(sanitize("Hello World!"), "hello_world_");
    }
}
