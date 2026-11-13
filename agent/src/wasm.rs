use std::path::Path;

use anyhow::Result;
use tracing::info;

/// Stub wasm reconciler: writes a dry-run config file describing desired functions.
pub fn apply_wasm_plan(workdir: &Path, functions: &[WasmFunctionPlan]) -> Result<()> {
    let config_dir = workdir.join("wasm");
    std::fs::create_dir_all(&config_dir)?;
    let config_path = config_dir.join("functions.json");
    let payload = serde_json::to_string_pretty(functions)?;
    std::fs::write(&config_path, payload)?;
    info!(path = %config_path.display(), count = functions.len(), "wasm plan applied (dry-run)");
    Ok(())
}

#[derive(Debug, serde::Serialize)]
pub struct WasmFunctionPlan {
    pub name: String,
    pub image_ref: String,
    pub fuel_limit: u64,
    pub memory_limit_mb: u32,
}
