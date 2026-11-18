use anyhow::{bail, Context, Result};
use reqwest::blocking::Client;
use reqwest::header::{AUTHORIZATION, CONTENT_TYPE};
use serde::Serialize;
use serde_json::Value;
use std::time::Duration;

use crate::config::normalize_api_base;

#[derive(Clone)]
pub struct ApiClient {
    http: Client,
    api_base: String,
    token: Option<String>,
}

impl ApiClient {
    pub fn new(api_base: &str, token: Option<String>) -> Result<Self> {
        let http = Client::builder()
            .user_agent(format!("norix-cli/{}", env!("CARGO_PKG_VERSION")))
            .timeout(Duration::from_secs(30))
            .build()
            .context("build HTTP client")?;
        Ok(Self {
            http,
            api_base: normalize_api_base(api_base),
            token,
        })
    }

    pub fn require_token(&self) -> Result<&str> {
        self.token
            .as_deref()
            .filter(|t| !t.is_empty())
            .context("authenticate with `norix login --token …` or set NORIX_TOKEN")
    }

    pub fn healthz(&self) -> Result<(u16, String)> {
        let url = format!("{}/healthz", self.api_base);
        let resp = self.http.get(url).send().context("GET /healthz")?;
        let status = resp.status().as_u16();
        let body = resp.text().unwrap_or_default();
        Ok((status, body.trim().to_string()))
    }

    pub fn connect_json<T: Serialize>(&self, procedure: &str, body: &T) -> Result<Value> {
        let path = if procedure.starts_with('/') {
            procedure.to_string()
        } else {
            format!("/{procedure}")
        };
        let url = format!("{}{}{path}", self.api_base, "/api");
        let mut req = self
            .http
            .post(url)
            .header(CONTENT_TYPE, "application/json")
            .header("Connect-Protocol-Version", "1")
            .json(body);
        if let Some(token) = &self.token {
            if !token.is_empty() {
                req = req.header(AUTHORIZATION, format!("Bearer {token}"));
            }
        }
        let resp = req.send().with_context(|| format!("POST {procedure}"))?;
        let status = resp.status();
        let text = resp.text().unwrap_or_default();
        if !status.is_success() {
            bail!("API {procedure}: HTTP {status}: {}", text.trim());
        }
        if text.trim().is_empty() {
            return Ok(Value::Object(Default::default()));
        }
        serde_json::from_str(&text).with_context(|| format!("decode {procedure} response"))
    }
}
