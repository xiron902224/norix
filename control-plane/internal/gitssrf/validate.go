// Package gitssrf blocks Server-Side Request Forgery via Git clone URLs (#4).
//
// Default policy: only https:// and ssh:// (plus scp-like git@host:path) are
// allowed. After DNS resolution, loopback, RFC1918, link-local, CGNAT, ULA and
// cloud-metadata-adjacent ranges are rejected unless allowlisted.
//
// NORIX_GIT_SSRF_ALLOWLIST: comma-separated hostnames and/or CIDRs that bypass
// the private-IP denylist (still require an allowed scheme).
package gitssrf

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

const AllowlistEnv = "NORIX_GIT_SSRF_ALLOWLIST"

// Resolver looks up host A/AAAA records. Tests inject fakes; production uses DefaultResolver.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// DefaultResolver is the process DNS resolver.
var DefaultResolver Resolver = net.DefaultResolver

// Policy controls validation. Zero value is fail-closed with built-in denylist.
type Policy struct {
	Resolver  Resolver
	Allowlist []string // hostnames (exact, lowercased) and/or CIDRs
}

// PolicyFromEnv builds a policy using DefaultResolver and NORIX_GIT_SSRF_ALLOWLIST.
func PolicyFromEnv() Policy {
	return Policy{
		Resolver:  DefaultResolver,
		Allowlist: parseAllowlist(os.Getenv(AllowlistEnv)),
	}
}

func parseAllowlist(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ValidateGitURL checks a Git remote URL under PolicyFromEnv.
func ValidateGitURL(raw string) error {
	return PolicyFromEnv().Validate(raw)
}

// Validate applies scheme + resolved-IP checks to raw.
func (p Policy) Validate(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("git url is required")
	}
	host, err := extractHost(raw)
	if err != nil {
		return err
	}
	if hostAllowlisted(host, p.Allowlist) {
		return nil
	}

	resolver := p.Resolver
	if resolver == nil {
		resolver = DefaultResolver
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Literal IP in the URL (https://127.0.0.1/… or https://[::1]/…).
	if ip := net.ParseIP(host); ip != nil {
		if err := checkIP(ip, p.Allowlist); err != nil {
			return fmt.Errorf("git url host %s: %w", host, err)
		}
		return nil
	}

	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("git url dns lookup %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("git url dns lookup %q: no addresses", host)
	}
	for _, addr := range addrs {
		if err := checkIP(addr.IP, p.Allowlist); err != nil {
			return fmt.Errorf("git url host %s resolves to blocked address %s: %w", host, addr.IP, err)
		}
	}
	return nil
}

func extractHost(raw string) (string, error) {
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "https://"):
		u, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("parse git url: %w", err)
		}
		if u.Host == "" {
			return "", fmt.Errorf("git url missing host")
		}
		host := u.Hostname()
		if host == "" {
			return "", fmt.Errorf("git url missing host")
		}
		return host, nil
	case strings.HasPrefix(lower, "http://"):
		return "", fmt.Errorf("git url scheme http is not allowed (use https)")
	case strings.HasPrefix(lower, "ssh://"):
		u, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("parse git url: %w", err)
		}
		host := u.Hostname()
		if host == "" {
			return "", fmt.Errorf("git url missing host")
		}
		return host, nil
	case strings.HasPrefix(lower, "file://"), strings.HasPrefix(lower, "git://"):
		return "", fmt.Errorf("git url scheme %s is not allowed", strings.SplitN(lower, ":", 2)[0])
	case strings.Contains(raw, "://"):
		scheme := strings.SplitN(lower, "://", 2)[0]
		return "", fmt.Errorf("git url scheme %s is not allowed", scheme)
	default:
		// SCP-like: git@host:path/repo.git
		host, ok := parseSCPHost(raw)
		if !ok {
			return "", fmt.Errorf("git url must be https://, ssh://, or scp-like user@host:path")
		}
		return host, nil
	}
}

func parseSCPHost(raw string) (string, bool) {
	// Reject paths that look like URLs we already handled.
	if strings.Contains(raw, "://") {
		return "", false
	}
	at := strings.LastIndex(raw, "@")
	if at < 0 {
		return "", false
	}
	rest := raw[at+1:]
	colon := strings.Index(rest, ":")
	if colon <= 0 {
		return "", false
	}
	host := rest[:colon]
	if host == "" || strings.Contains(host, "/") {
		return "", false
	}
	// Bracketed IPv6: user@[::1]:repo.git (uncommon but possible)
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	return host, true
}

func hostAllowlisted(host string, allowlist []string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	for _, entry := range allowlist {
		if entry == h {
			return true
		}
	}
	return false
}

func checkIP(ip net.IP, allowlist []string) error {
	if ip == nil {
		return fmt.Errorf("nil ip")
	}
	// Normalize IPv4-mapped IPv6.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, entry := range allowlist {
		if _, network, err := net.ParseCIDR(entry); err == nil && network.Contains(ip) {
			return nil
		}
		if parsed := net.ParseIP(entry); parsed != nil && parsed.Equal(ip) {
			return nil
		}
	}
	if blocked, reason := isBlockedIP(ip); blocked {
		return fmt.Errorf("%s", reason)
	}
	return nil
}

func isBlockedIP(ip net.IP) (bool, string) {
	if ip.IsLoopback() {
		return true, "loopback addresses are not allowed"
	}
	if ip.IsUnspecified() {
		return true, "unspecified addresses are not allowed"
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true, "link-local addresses are not allowed"
	}
	if ip.IsPrivate() {
		return true, "private (RFC1918 / ULA) addresses are not allowed"
	}
	if ip.IsMulticast() {
		return true, "multicast addresses are not allowed"
	}
	// CGNAT 100.64.0.0/10 (not covered by IsPrivate in all Go versions historically;
	// keep explicit for SSRF).
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true, "shared address space (CGNAT 100.64.0.0/10) is not allowed"
		}
		// Explicit metadata / link-local neighbor (also link-local, belt-and-suspenders).
		if v4[0] == 169 && v4[1] == 254 {
			return true, "link-local / cloud metadata addresses are not allowed"
		}
	}
	return false, ""
}
