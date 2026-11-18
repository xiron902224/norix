use std::fs;
use std::path::PathBuf;

use anyhow::{Context, Result};

pub fn config_dir() -> Result<PathBuf> {
    let home = dirs::home_dir().context("resolve home directory")?;
    Ok(home.join(".norix"))
}

pub fn token_path() -> Result<PathBuf> {
    Ok(config_dir()?.join("token"))
}

pub fn read_token() -> Option<String> {
    let path = token_path().ok()?;
    let raw = fs::read_to_string(path).ok()?;
    let token = raw.trim().to_string();
    if token.is_empty() {
        None
    } else {
        Some(token)
    }
}

pub fn write_token(token: &str) -> Result<PathBuf> {
    let dir = config_dir()?;
    fs::create_dir_all(&dir).with_context(|| format!("create {}", dir.display()))?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let _ = fs::set_permissions(&dir, fs::Permissions::from_mode(0o700));
    }
    let path = dir.join("token");
    fs::write(&path, format!("{}\n", token.trim()))
        .with_context(|| format!("write {}", path.display()))?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let _ = fs::set_permissions(&path, fs::Permissions::from_mode(0o600));
    }
    Ok(path)
}

pub fn normalize_api_base(raw: &str) -> String {
    raw.trim().trim_end_matches('/').to_string()
}
