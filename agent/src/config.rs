use std::env;
use std::path::PathBuf;

use anyhow::{bail, Context, Result};

#[derive(Debug, Clone)]
pub struct Config {
    pub control_plane_url: String,
    pub join_token: Option<String>,
    pub node_name: String,
    pub workdir: PathBuf,
    pub heartbeat_secs: u64,
    pub agent_version: String,
}

impl Config {
    pub fn from_env() -> Result<Self> {
        let control_plane_url = env::var("NORIX_CONTROL_PLANE_URL")
            .unwrap_or_else(|_| "http://127.0.0.1:8080".into())
            .trim_end_matches('/')
            .to_string();
        let join_token = env::var("NORIX_JOIN_TOKEN")
            .ok()
            .map(|value| value.trim().to_string())
            .filter(|value| !value.is_empty());
        let node_name = env::var("NORIX_NODE_NAME").unwrap_or_else(|_| {
            hostname::get()
                .ok()
                .and_then(|value| value.into_string().ok())
                .unwrap_or_else(|| "norix-node".into())
        });
        let workdir = PathBuf::from(
            env::var("NORIX_AGENT_WORKDIR").unwrap_or_else(|_| "./.agent".into()),
        );
        let heartbeat_secs = env::var("NORIX_HEARTBEAT_SECS")
            .ok()
            .and_then(|value| value.parse().ok())
            .unwrap_or(10);
        if heartbeat_secs == 0 {
            bail!("NORIX_HEARTBEAT_SECS must be >= 1");
        }
        Ok(Self {
            control_plane_url,
            join_token,
            node_name,
            workdir,
            heartbeat_secs,
            agent_version: env!("CARGO_PKG_VERSION").into(),
        })
    }

    pub fn api_base(&self) -> String {
        format!("{}/api", self.control_plane_url)
    }

    pub fn state_path(&self) -> PathBuf {
        self.workdir.join("node-state.json")
    }

    pub fn ensure_workdir(&self) -> Result<()> {
        std::fs::create_dir_all(&self.workdir)
            .with_context(|| format!("create workdir {}", self.workdir.display()))
    }

    pub fn validate_for_startup(&self) -> Result<()> {
        if self.control_plane_url.trim().is_empty() {
            bail!("NORIX_CONTROL_PLANE_URL cannot be empty");
        }
        if self.join_token.is_none() && !self.state_path().exists() {
            bail!(
                "missing NORIX_JOIN_TOKEN and no saved node state at {}. Generate a token in the dashboard or restore the agent state.",
                self.state_path().display()
            );
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::Config;

    #[test]
    fn startup_validation_requires_join_token_or_saved_state() {
        let cfg = Config {
            control_plane_url: "http://127.0.0.1:8080".to_string(),
            join_token: None,
            node_name: "node-1".to_string(),
            workdir: std::env::temp_dir().join("norix-agent-config-test"),
            heartbeat_secs: 10,
            agent_version: "test".to_string(),
        };

        let result = cfg.validate_for_startup();
        assert!(result.is_err());
    }
}
