//! Pure helpers for the compose override (#144–#147).
//!
//! Docker Compose merges sequence fields such as `ports` by appending, so a
//! JSON override cannot unpublish a database. The override is YAML and uses
//! `ports: !reset []` for every service the user did not mark public.

use std::collections::{BTreeMap, BTreeSet};
use std::path::{Component, Path, PathBuf};

use anyhow::{bail, Result};
use serde_json::Value;

use crate::client::ComposeVolume;

#[derive(Debug, Clone)]
pub struct ServiceModel {
    pub name: String,
    pub aliases: Vec<String>,
    pub container_port: i32,
}

#[derive(Debug, Clone)]
pub struct ComposeModel {
    pub services: Vec<ServiceModel>,
    pub external_networks: Vec<String>,
    pub volumes: Vec<ComposeVolume>,
}

/// `norix-p-` plus the project UUID hex. Every stack in the project joins it.
pub fn project_network_name(project_id: &str) -> Result<String> {
    let hex: String = project_id
        .chars()
        .filter(|c| c.is_ascii_hexdigit())
        .map(|c| c.to_ascii_lowercase())
        .collect();
    if hex.len() < 8 || hex.len() > 32 {
        bail!("invalid project id");
    }
    Ok(format!("norix-p-{hex}"))
}

/// Override YAML (#143–#146). `compose_project` is the `-p` value, e.g. `norix-<hex12>`.
/// Services that are not public lose their host ports with `!reset`.
pub fn build_override_yaml(
    model: &ComposeModel,
    stack_id: &str,
    project_id: &str,
    compose_project: &str,
    public_services: &[String],
) -> Result<String> {
    let network = project_network_name(project_id)?;
    if !is_dns_name(&network) || !is_dns_name(compose_project) {
        bail!("invalid project network or compose project name");
    }
    let mut out = String::new();
    // External networks the user's own file declares are created by the agent before
    // `up` (see ComposeModel::external_networks). The override only adds the project one.
    out.push_str("networks:\n  norix_project:\n    external: true\n    name: ");
    out.push_str(&network);
    out.push_str("\nservices:\n");
    let public: BTreeSet<&str> = public_services.iter().map(String::as_str).collect();
    for service in &model.services {
        if !is_dns_name(&service.name) {
            bail!("invalid compose service name {}", service.name);
        }
        out.push_str("  ");
        out.push_str(&service.name);
        out.push_str(":\n    labels:\n      norix.stack_id: \"");
        out.push_str(stack_id);
        out.push_str("\"\n      norix.project_id: \"");
        out.push_str(project_id);
        out.push_str("\"\n    networks:\n      norix_project:\n        aliases:\n");
        let mut aliases: Vec<String> = service
            .aliases
            .iter()
            .filter(|alias| is_dns_name(alias))
            .cloned()
            .collect();
        aliases.push(unique_service_alias(compose_project, &service.name));
        aliases.sort();
        aliases.dedup();
        for alias in &aliases {
            out.push_str("          - ");
            out.push_str(alias);
            out.push('\n');
        }
        if !public.contains(service.name.as_str()) {
            out.push_str("    ports: !reset []\n");
        }
    }
    Ok(out)
}

pub fn parse_compose_config(raw: &str) -> Result<ComposeModel> {
    let value: Value = serde_json::from_str(raw.trim()).or_else(|_| {
        // `compose config` may print warnings before the JSON document.
        let start = raw.find('{').ok_or_else(|| anyhow::anyhow!("compose config is not JSON"))?;
        serde_json::from_str(&raw[start..]).map_err(|err| anyhow::anyhow!("parse compose config: {err}"))
    })?;
    let services_obj = value
        .get("services")
        .and_then(Value::as_object)
        .ok_or_else(|| anyhow::anyhow!("compose config has no services"))?;
    let mut services = Vec::new();
    let mut volumes = Vec::new();
    for (name, spec) in services_obj {
        let mut aliases = BTreeSet::new();
        aliases.insert(name.clone());
        if let Some(networks) = spec.get("networks").and_then(Value::as_object) {
            for net in networks.values() {
                if let Some(list) = net.get("aliases").and_then(Value::as_array) {
                    for alias in list {
                        if let Some(alias) = alias.as_str() {
                            aliases.insert(alias.to_string());
                        }
                    }
                }
            }
        }
        if let Some(items) = spec.get("volumes").and_then(Value::as_array) {
            for item in items {
                if let Some(volume) = volume_from_config(item) {
                    volumes.push(volume);
                }
            }
        }
        services.push(ServiceModel {
            name: name.clone(),
            aliases: aliases.into_iter().collect(),
            container_port: container_port(spec),
        });
    }
    services.sort_by(|a, b| a.name.cmp(&b.name));
    if services.is_empty() {
        bail!("compose file has no services");
    }
    if let Some(named) = value.get("volumes").and_then(Value::as_object) {
        for name in named.keys() {
            volumes.push(ComposeVolume {
                name: name.clone(),
                kind: "named".into(),
                source: String::new(),
            });
        }
    }
    volumes.sort_by(|a, b| a.name.cmp(&b.name).then(a.kind.cmp(&b.kind)));
    volumes.dedup_by(|a, b| a.name == b.name && a.kind == b.kind);
    let mut external_networks = Vec::new();
    if let Some(networks) = value.get("networks").and_then(Value::as_object) {
        for (key, spec) in networks {
            if spec.get("external").and_then(Value::as_bool) != Some(true) {
                continue;
            }
            let name = spec
                .get("name")
                .and_then(Value::as_str)
                .filter(|name| !name.is_empty())
                .unwrap_or(key);
            if is_dns_name(name) {
                external_networks.push(name.to_string());
            }
        }
    }
    external_networks.sort();
    external_networks.dedup();
    Ok(ComposeModel {
        services,
        external_networks,
        volumes,
    })
}

fn container_port(spec: &Value) -> i32 {
    if let Some(ports) = spec.get("ports").and_then(Value::as_array) {
        for port in ports {
            let proto = port.get("protocol").and_then(Value::as_str).unwrap_or("tcp");
            if proto != "tcp" {
                continue;
            }
            if let Some(target) = port.get("target").and_then(Value::as_i64) {
                if (1..=65535).contains(&target) {
                    return target as i32;
                }
            }
        }
    }
    if let Some(expose) = spec.get("expose").and_then(Value::as_array) {
        for item in expose {
            if let Some(text) = item.as_str() {
                let number = text.split('/').next().unwrap_or("").parse::<i32>().unwrap_or(0);
                if (1..=65535).contains(&number) {
                    return number;
                }
            }
        }
    }
    0
}

fn volume_from_config(item: &Value) -> Option<ComposeVolume> {
    if let Some(text) = item.as_str() {
        let (source, _) = text.split_once(':')?;
        let kind = if source.starts_with('/') || source.starts_with('.') {
            "bind"
        } else {
            "named"
        };
        return Some(ComposeVolume {
            name: source.to_string(),
            kind: kind.into(),
            source: source.to_string(),
        });
    }
    let kind = item.get("type").and_then(Value::as_str).unwrap_or("volume");
    let source = item.get("source").and_then(Value::as_str).unwrap_or("").to_string();
    if source.is_empty() {
        return None;
    }
    let kind = if kind == "bind" { "bind" } else { "named" };
    Some(ComposeVolume {
        name: source.clone(),
        kind: kind.into(),
        source,
    })
}

/// Names referenced as `${NAME:?...}`. Compose treats those as required.
pub fn required_var_names(text: &str) -> Vec<String> {
    interpolation_names(text)
        .into_iter()
        .filter(|(_, required)| *required)
        .map(|(name, _)| name)
        .collect()
}

/// `SERVICE_PASSWORD_*` and `SERVICE_BASE64_*` referenced anywhere in the file.
pub fn generated_secret_names(text: &str) -> Vec<String> {
    interpolation_names(text)
        .into_iter()
        .map(|(name, _)| name)
        .filter(|name| {
            name.starts_with("SERVICE_PASSWORD_") || name.starts_with("SERVICE_BASE64_")
        })
        .collect()
}

fn interpolation_names(text: &str) -> Vec<(String, bool)> {
    let mut out = Vec::new();
    let bytes = text.as_bytes();
    let mut i = 0;
    while i + 2 < bytes.len() {
        if bytes[i] == b'$' && bytes[i + 1] == b'{' {
            let rest = &text[i + 2..];
            let end = rest.find('}').unwrap_or(rest.len());
            let expr = &rest[..end];
            let name_end = expr
                .find(|c: char| !(c.is_ascii_alphanumeric() || c == '_'))
                .unwrap_or(expr.len());
            let name = &expr[..name_end];
            if !name.is_empty() && (name.as_bytes()[0].is_ascii_alphabetic() || name.as_bytes()[0] == b'_')
            {
                let required = expr[name_end..].starts_with('?') || expr[name_end..].starts_with(":?");
                out.push((name.to_string(), required));
            }
            i += 2 + end;
            continue;
        }
        i += 1;
    }
    out.sort();
    out.dedup();
    out
}

/// Renders a compose `.env` file. Values are single-quoted so `$` is literal.
/// Values with a single quote use double quotes; `$` is escaped as `$$` there.
pub fn format_env(vars: &BTreeMap<String, String>) -> String {
    let mut out = String::new();
    for (name, value) in vars {
        if !is_env_name(name) || value.contains('\n') || value.contains('\0') || value.contains('\r') {
            continue;
        }
        out.push_str(name);
        out.push('=');
        if !value.contains('\'') {
            out.push('\'');
            out.push_str(value);
            out.push('\'');
        } else {
            out.push('"');
            out.push_str(
                &value
                    .replace('\\', "\\\\")
                    .replace('"', "\\\"")
                    .replace('$', "$$"),
            );
            out.push('"');
        }
        out.push('\n');
    }
    out
}

fn is_env_name(name: &str) -> bool {
    let bytes = name.as_bytes();
    !bytes.is_empty()
        && (bytes[0].is_ascii_alphabetic() || bytes[0] == b'_')
        && bytes.iter().all(|c| c.is_ascii_alphanumeric() || *c == b'_')
}

const ALNUM: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
const B64: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

/// Random bytes from the kernel CSPRNG. Fails closed: no fallback to weak sources.
pub fn random_bytes(len: usize) -> Result<Vec<u8>> {
    use std::io::Read;
    let mut buf = vec![0u8; len];
    let mut file = std::fs::File::open("/dev/urandom")
        .map_err(|error| anyhow::anyhow!("open /dev/urandom: {error}"))?;
    file.read_exact(&mut buf)
        .map_err(|error| anyhow::anyhow!("read /dev/urandom: {error}"))?;
    Ok(buf)
}

/// `SERVICE_PASSWORD_*`: 32 alphanumeric characters (about 190 bits).
pub fn generate_password() -> Result<String> {
    let bytes = random_bytes(32)?;
    Ok(bytes.iter().map(|b| ALNUM[(*b as usize) % ALNUM.len()] as char).collect())
}

/// `SERVICE_BASE64_*`: standard base64 of 32 random bytes (no line breaks).
pub fn generate_base64_secret() -> Result<String> {
    let bytes = random_bytes(32)?;
    Ok(base64_encode(&bytes))
}

pub fn base64_encode(input: &[u8]) -> String {
    let mut out = String::with_capacity(input.len().div_ceil(3) * 4);
    for chunk in input.chunks(3) {
        let b0 = chunk[0] as u32;
        let b1 = chunk.get(1).copied().unwrap_or(0) as u32;
        let b2 = chunk.get(2).copied().unwrap_or(0) as u32;
        let n = (b0 << 16) | (b1 << 8) | b2;
        out.push(B64[((n >> 18) & 63) as usize] as char);
        out.push(B64[((n >> 12) & 63) as usize] as char);
        out.push(if chunk.len() > 1 { B64[((n >> 6) & 63) as usize] as char } else { '=' });
        out.push(if chunk.len() > 2 { B64[(n & 63) as usize] as char } else { '=' });
    }
    out
}

/// Alias used on the project network: `<compose project>-<service>`.
/// The edge upstream uses the same name, so two repos with `api` do not collide.
pub fn unique_service_alias(compose_project: &str, service: &str) -> String {
    format!("{compose_project}-{service}")
}

pub fn redact_secrets(text: &str, secrets: &[String]) -> String {
    let mut out = text.to_string();
    for secret in secrets {
        if secret.len() >= 6 {
            out = out.replace(secret, "***");
        }
    }
    out
}

/// True when a bind source is outside the checkout and the allowlist (#147).
pub fn bind_rejected(source: &str, checkout: &Path, allowlist: &[String]) -> bool {
    let candidate = Path::new(source);
    if !candidate.is_absolute() {
        return true;
    }
    if path_within(candidate, checkout) {
        return false;
    }
    for extra in allowlist {
        let extra = extra.trim();
        if extra.is_empty() {
            continue;
        }
        if path_within(candidate, Path::new(extra)) {
            return false;
        }
    }
    true
}

fn path_within(candidate: &Path, root: &Path) -> bool {
    let root = root.canonicalize().unwrap_or_else(|_| normalize_lex(root));
    if let Ok(real) = candidate.canonicalize() {
        return real.starts_with(&root);
    }
    normalize_lex(candidate).starts_with(&root)
}

fn normalize_lex(path: &Path) -> PathBuf {
    let mut out = PathBuf::new();
    for component in path.components() {
        match component {
            Component::ParentDir => {
                out.pop();
            }
            Component::CurDir => {}
            other => out.push(other),
        }
    }
    out
}

fn is_dns_name(value: &str) -> bool {
    let bytes = value.as_bytes();
    !bytes.is_empty()
        && bytes.len() <= 128
        && bytes[0].is_ascii_alphanumeric()
        && bytes.iter().all(|c| c.is_ascii_alphanumeric() || *c == b'_' || *c == b'-' || *c == b'.')
}

pub fn bind_allowlist_from_env(raw: &str) -> Vec<String> {
    raw.split(',')
        .map(|part| part.trim().to_string())
        .filter(|part| !part.is_empty())
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn project_network_uses_the_project_uuid() {
        let name = project_network_name("3F2504E0-4F89-11D3-9A0C-0305E82C3301").unwrap();
        assert_eq!(name, "norix-p-3f2504e04f8911d39a0c0305e82c3301");
        assert!(project_network_name("short").is_err());
    }

    #[test]
    fn override_resets_ports_and_keeps_aliases() {
        let model = ComposeModel {
            services: vec![ServiceModel {
                name: "db".into(),
                aliases: vec!["db".into(), "postgres-develop".into()],
                container_port: 5432,
            }],
            external_networks: vec![],
            volumes: vec![],
        };
        let project = "3f2504e0-4f89-11d3-9a0c-0305e82c3301";
        let yaml = build_override_yaml(&model, "stack-1", project, "norix-aaaaaaaaaaaa", &[]).unwrap();
        assert!(yaml.contains("name: norix-p-3f2504e04f8911d39a0c0305e82c3301"));
        assert!(yaml.contains("ports: !reset []"));
        assert!(yaml.contains("- postgres-develop"));
        assert!(yaml.contains("- norix-aaaaaaaaaaaa-db"));
        let public = build_override_yaml(&model, "stack-1", project, "norix-aaaaaaaaaaaa", &["db".into()]).unwrap();
        assert!(!public.contains("!reset"));
    }

    #[test]
    fn parses_external_networks_aliases_and_ports() {
        let raw = r#"{"services":{"api":{"networks":{"backend":{"aliases":["api-develop"]}},"ports":[{"target":8080,"protocol":"tcp"}]}},"networks":{"backend":{"external":true,"name":"shared_backend"},"default":{"external":false}}}"#;
        let model = parse_compose_config(raw).unwrap();
        assert_eq!(model.external_networks, vec!["shared_backend".to_string()]);
        assert_eq!(model.services[0].container_port, 8080);
        assert!(model.services[0].aliases.iter().any(|a| a == "api-develop"));
        assert!(model.services[0].aliases.iter().any(|a| a == "api"));
    }

    #[test]
    fn required_and_generated_names() {
        let text = "image: db\nenvironment:\n  PASSWORD: ${DB_PASSWORD:?required}\n  OPT: ${OPTIONAL:-x}\n  GEN: ${SERVICE_PASSWORD_DB}\n";
        assert_eq!(required_var_names(text), vec!["DB_PASSWORD".to_string()]);
        assert_eq!(generated_secret_names(text), vec!["SERVICE_PASSWORD_DB".to_string()]);
    }

    #[test]
    fn bind_outside_checkout_is_rejected() {
        let dir = std::env::temp_dir().join(format!("norix-bind-{}", std::process::id()));
        std::fs::create_dir_all(&dir).unwrap();
        let inside = dir.join("data");
        std::fs::create_dir_all(&inside).unwrap();
        assert!(!bind_rejected(&inside.to_string_lossy(), &dir, &[]));
        assert!(bind_rejected("/var/run/docker.sock", &dir, &[]));
        assert!(!bind_rejected("/var/run/docker.sock", &dir, &["/var/run".into()]));
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn redact_replaces_secret_values() {
        let out = redact_secrets("password=supersecret", &["supersecret".into()]);
        assert_eq!(out, "password=***");
        assert_eq!(redact_secrets("short=abcd", &["abcd".into()]), "short=abcd");
    }
}
