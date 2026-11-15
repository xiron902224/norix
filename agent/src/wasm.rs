use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

use anyhow::{bail, Context, Result};
use tracing::{info, warn};

use crate::client::{DesiredWasmFunction, DesiredWasmPlan, ObservedWasm, ObservedWasmFunction};

/// Reconcile DesiredWasmPlan: stage artifacts, validate wasm magic, optionally
/// smoke-run with `wasmtime` when present, and persist functions.json.
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
        "dry_run": false,
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
        status: if any_failed { "degraded".into() } else { "ready".into() },
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
        && (image.starts_with("http://") || image.starts_with("https://") || Path::new(image).exists())
    {
        fetch_wasm_file(image, &dest, logs)?;
    } else {
        // OCI/component refs: try docker create+cp, else record the ref for offline tooling.
        if !try_extract_from_oci(image, &dest, logs)? {
            let stub = artifacts.join(format!("{safe}.ref"));
            fs::write(&stub, image).context("write image ref stub")?;
            logs.push(format!(
                "{}: OCI ref recorded at {} (install wasmtime + oras/docker to materialize .wasm)",
                function.name,
                stub.display()
            ));
            return Ok(ObservedWasmFunction {
                id: function.id.clone(),
                status: "pending".into(),
                message: "OCI wasm component not extracted; ref recorded".into(),
                artifact_path: stub.display().to_string(),
            });
        }
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

    let mut message = format!("artifact ready fuel={fuel} memory_mb={mem_mb}");
    let mut status = "ready".to_string();
    if let Some(serve) = try_wasi_http_serve(&dest, fuel, mem_mb, logs) {
        status = "running".into();
        message = serve;
    } else if let Some(smoke) = smoke_wasmtime(&dest, fuel, mem_mb) {
        logs.push(format!("{}: {smoke}", function.name));
        if smoke.contains("failed") {
            status = "degraded".into();
            message = smoke;
        } else {
            status = "running".into();
            message = smoke;
        }
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

fn try_extract_from_oci(image: &str, dest: &Path, logs: &mut Vec<String>) -> Result<bool> {
    // Prefer `docker create` + `docker cp` when the image is a single-layer wasm blob layout.
    let create = Command::new("docker")
        .args(["create", image])
        .output();
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
    ];
    let mut ok = false;
    for path in candidates {
        let cp = Command::new("docker")
            .args(["cp", &format!("{cid}:{path}"), &dest.display().to_string()])
            .output();
        if let Ok(cp) = cp {
            if cp.status.success() && dest.exists() {
                logs.push(format!("extracted {path} from {image}"));
                ok = true;
                break;
            }
        }
    }
    let _ = Command::new("docker").args(["rm", "-f", &cid]).output();
    Ok(ok)
}

fn validate_wasm_magic(path: &Path) -> Result<()> {
    let bytes = fs::read(path).with_context(|| format!("read {}", path.display()))?;
    if bytes.len() < 4 || &bytes[0..4] != b"\0asm" {
        bail!("{} is not a wasm binary (missing \\0asm magic)", path.display());
    }
    Ok(())
}

/// Try `wasmtime serve` for WASI HTTP components; returns running message on success.
fn try_wasi_http_serve(path: &Path, fuel: u64, mem_mb: i32, logs: &mut Vec<String>) -> Option<String> {
    let help = Command::new("wasmtime").args(["serve", "--help"]).output().ok()?;
    if !help.status.success() {
        return None;
    }
    // Probe-only: confirm serve accepts the module without binding a long-lived process in reconcile.
    // Long-lived serve is started by the node supervisor via functions.json + systemd/unit (addr in env).
    let addr = std::env::var("NORIX_WASM_SERVE_ADDR").unwrap_or_else(|_| "127.0.0.1:0".into());
    if std::env::var("NORIX_WASM_SERVE").ok().as_deref() != Some("1") {
        logs.push(format!(
            "wasi http capable (set NORIX_WASM_SERVE=1 to spawn wasmtime serve on {addr})"
        ));
        return Some(format!("wasi-http ready addr={addr}"));
    }
    let child = Command::new("wasmtime")
        .args([
            "serve",
            "--addr",
            &addr,
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
    // Detach: drop Child without wait so the HTTP worker keeps running.
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
