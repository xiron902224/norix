//! Block SSRF via Git clone URLs (parity with control-plane `internal/gitssrf`, issue #4).
//!
//! Allowed schemes: `https://`, `ssh://`, and scp-like `user@host:path`.
//! After DNS lookup, loopback / RFC1918 / link-local / CGNAT / ULA are rejected
//! unless listed in `NORIX_GIT_SSRF_ALLOWLIST` (hostnames or CIDRs).

use std::env;
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr, ToSocketAddrs};
use std::str::FromStr;

use anyhow::{bail, Context, Result};

const ALLOWLIST_ENV: &str = "NORIX_GIT_SSRF_ALLOWLIST";

pub fn validate_git_url(raw: &str) -> Result<()> {
    let raw = raw.trim();
    if raw.is_empty() {
        bail!("git url is required");
    }
    let allowlist = parse_allowlist(&env::var(ALLOWLIST_ENV).unwrap_or_default());
    let host = extract_host(raw)?;
    if host_allowlisted(&host, &allowlist) {
        return Ok(());
    }

    if let Ok(ip) = IpAddr::from_str(&host) {
        return check_ip(ip, &allowlist).with_context(|| format!("git url host {host}"));
    }

    // Resolve via std DNS (A/AAAA). ToSocketAddrs needs a port; 443 is unused for the check.
    let addrs: Vec<_> = format!("{host}:443")
        .to_socket_addrs()
        .with_context(|| format!("git url dns lookup {host:?}"))?
        .map(|a| a.ip())
        .collect();
    if addrs.is_empty() {
        bail!("git url dns lookup {host:?}: no addresses");
    }
    for ip in addrs {
        check_ip(ip, &allowlist).with_context(|| {
            format!("git url host {host} resolves to blocked address {ip}")
        })?;
    }
    Ok(())
}

fn parse_allowlist(raw: &str) -> Vec<String> {
    raw.split(',')
        .map(|s| s.trim().to_ascii_lowercase())
        .filter(|s| !s.is_empty())
        .collect()
}

fn host_allowlisted(host: &str, allowlist: &[String]) -> bool {
    let h = host.to_ascii_lowercase();
    allowlist.iter().any(|e| e == &h)
}

fn extract_host(raw: &str) -> Result<String> {
    let lower = raw.to_ascii_lowercase();
    if lower.starts_with("https://") {
        return host_from_url(raw);
    }
    if lower.starts_with("http://") {
        bail!("git url scheme http is not allowed (use https)");
    }
    if lower.starts_with("ssh://") {
        return host_from_url(raw);
    }
    if lower.starts_with("file://") || lower.starts_with("git://") {
        let scheme = lower.split(':').next().unwrap_or("unknown");
        bail!("git url scheme {scheme} is not allowed");
    }
    if lower.contains("://") {
        let scheme = lower.split("://").next().unwrap_or("unknown");
        bail!("git url scheme {scheme} is not allowed");
    }
    parse_scp_host(raw)
}

fn host_from_url(raw: &str) -> Result<String> {
    // Minimal parse without pulling url crate: scheme://[userinfo@]host[:port][/…]
    let rest = raw
        .split_once("://")
        .map(|(_, r)| r)
        .ok_or_else(|| anyhow::anyhow!("parse git url: missing scheme"))?;
    let authority = rest.split('/').next().unwrap_or("");
    let hostport = authority.rsplit('@').next().unwrap_or("");
    let host = if hostport.starts_with('[') {
        hostport
            .trim_start_matches('[')
            .split(']')
            .next()
            .unwrap_or("")
            .to_string()
    } else {
        hostport.split(':').next().unwrap_or("").to_string()
    };
    if host.is_empty() {
        bail!("git url missing host");
    }
    Ok(host)
}

fn parse_scp_host(raw: &str) -> Result<String> {
    let at = raw
        .rfind('@')
        .ok_or_else(|| anyhow::anyhow!("git url must be https://, ssh://, or scp-like user@host:path"))?;
    let rest = &raw[at + 1..];
    let colon = rest
        .find(':')
        .filter(|&i| i > 0)
        .ok_or_else(|| anyhow::anyhow!("git url must be https://, ssh://, or scp-like user@host:path"))?;
    let mut host = rest[..colon].to_string();
    if host.contains('/') {
        bail!("git url must be https://, ssh://, or scp-like user@host:path");
    }
    if host.starts_with('[') && host.ends_with(']') {
        host = host.trim_matches(|c| c == '[' || c == ']').to_string();
    }
    Ok(host)
}

fn check_ip(ip: IpAddr, allowlist: &[String]) -> Result<()> {
    for entry in allowlist {
        if let Ok(allowed) = IpAddr::from_str(entry) {
            if allowed == ip {
                return Ok(());
            }
        }
        if cidr_contains(entry, ip) {
            return Ok(());
        }
    }
    if let Some(reason) = blocked_reason(ip) {
        bail!("{reason}");
    }
    Ok(())
}

fn cidr_contains(cidr: &str, ip: IpAddr) -> bool {
    let Some((base, bits)) = cidr.split_once('/') else {
        return false;
    };
    let Ok(prefix_len) = bits.parse::<u8>() else {
        return false;
    };
    match (IpAddr::from_str(base), ip) {
        (Ok(IpAddr::V4(base_ip)), IpAddr::V4(target)) => {
            if prefix_len > 32 {
                return false;
            }
            let mask = if prefix_len == 0 {
                0u32
            } else {
                u32::MAX << (32 - prefix_len)
            };
            (u32::from(base_ip) & mask) == (u32::from(target) & mask)
        }
        (Ok(IpAddr::V6(base_ip)), IpAddr::V6(target)) => {
            if prefix_len > 128 {
                return false;
            }
            let base_bytes = base_ip.octets();
            let target_bytes = target.octets();
            let full = (prefix_len / 8) as usize;
            let rem = prefix_len % 8;
            if base_bytes[..full] != target_bytes[..full] {
                return false;
            }
            if rem == 0 {
                return true;
            }
            let mask = 0xffu8 << (8 - rem);
            (base_bytes[full] & mask) == (target_bytes[full] & mask)
        }
        _ => false,
    }
}

fn blocked_reason(ip: IpAddr) -> Option<&'static str> {
    match ip {
        IpAddr::V4(v4) => blocked_v4(v4),
        IpAddr::V6(v6) => {
            if let Some(v4) = v6.to_ipv4_mapped() {
                return blocked_v4(v4);
            }
            blocked_v6(v6)
        }
    }
}

fn blocked_v4(ip: Ipv4Addr) -> Option<&'static str> {
    if ip.is_loopback() {
        return Some("loopback addresses are not allowed");
    }
    if ip.is_unspecified() {
        return Some("unspecified addresses are not allowed");
    }
    if ip.is_link_local() {
        return Some("link-local / cloud metadata addresses are not allowed");
    }
    if ip.is_private() {
        return Some("private (RFC1918 / ULA) addresses are not allowed");
    }
    if ip.is_multicast() {
        return Some("multicast addresses are not allowed");
    }
    let octets = ip.octets();
    // CGNAT 100.64.0.0/10
    if octets[0] == 100 && (64..=127).contains(&octets[1]) {
        return Some("shared address space (CGNAT 100.64.0.0/10) is not allowed");
    }
    None
}

fn blocked_v6(ip: Ipv6Addr) -> Option<&'static str> {
    if ip.is_loopback() {
        return Some("loopback addresses are not allowed");
    }
    if ip.is_unspecified() {
        return Some("unspecified addresses are not allowed");
    }
    if (ip.segments()[0] & 0xffc0) == 0xfe80 {
        return Some("link-local addresses are not allowed");
    }
    // ULA fc00::/7
    if (ip.segments()[0] & 0xfe00) == 0xfc00 {
        return Some("private (RFC1918 / ULA) addresses are not allowed");
    }
    if ip.is_multicast() {
        return Some("multicast addresses are not allowed");
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rejects_http_and_file() {
        assert!(validate_git_url("http://github.com/a/b.git").is_err());
        assert!(validate_git_url("file:///tmp/repo").is_err());
    }

    #[test]
    fn rejects_literal_private_and_metadata() {
        assert!(validate_git_url("https://127.0.0.1/repo.git").is_err());
        assert!(validate_git_url("https://10.0.0.1/repo.git").is_err());
        assert!(validate_git_url("https://169.254.169.254/latest").is_err());
        assert!(validate_git_url("https://[::1]/repo.git").is_err());
    }

    #[test]
    fn extracts_scp_host() {
        assert_eq!(
            extract_host("git@github.com:org/repo.git").unwrap(),
            "github.com"
        );
    }
}
