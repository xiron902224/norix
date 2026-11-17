package gitssrf

import (
	"context"
	"net"
	"strings"
	"testing"
)

type mapResolver map[string][]net.IP

func (m mapResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	_ = ctx
	ips, ok := m[strings.ToLower(host)]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	out := make([]net.IPAddr, 0, len(ips))
	for _, ip := range ips {
		out = append(out, net.IPAddr{IP: ip})
	}
	return out, nil
}

func TestValidate_Table(t *testing.T) {
	resolver := mapResolver{
		"github.com":       {net.ParseIP("140.82.112.3")},
		"evil.internal":    {net.ParseIP("10.0.0.5")},
		"loop.example":     {net.ParseIP("127.0.0.1")},
		"link.example":     {net.ParseIP("169.254.169.254")},
		"cgnat.example":    {net.ParseIP("100.64.1.2")},
		"ula.example":      {net.ParseIP("fd12:3456:789a::1")},
		"mixed.example":    {net.ParseIP("1.2.3.4"), net.ParseIP("192.168.1.1")},
		"allowme.example":  {net.ParseIP("10.1.2.3")},
	}
	base := Policy{Resolver: resolver}

	tests := []struct {
		name      string
		url       string
		allowlist []string
		wantErr   string // substring; empty = ok
	}{
		{name: "https public ok", url: "https://github.com/org/repo.git"},
		{name: "ssh public ok", url: "ssh://git@github.com/org/repo.git"},
		{name: "scp-like public ok", url: "git@github.com:org/repo.git"},

		{name: "http rejected", url: "http://github.com/org/repo.git", wantErr: "http is not allowed"},
		{name: "file rejected", url: "file:///tmp/repo", wantErr: "file is not allowed"},
		{name: "git scheme rejected", url: "git://github.com/org/repo.git", wantErr: "git is not allowed"},

		{name: "literal loopback", url: "https://127.0.0.1/repo.git", wantErr: "loopback"},
		{name: "literal rfc1918 10", url: "https://10.0.0.1/repo.git", wantErr: "private"},
		{name: "literal rfc1918 192", url: "https://192.168.0.9/x.git", wantErr: "private"},
		{name: "literal rfc1918 172", url: "https://172.16.5.5/x.git", wantErr: "private"},
		{name: "literal metadata", url: "https://169.254.169.254/latest/meta-data", wantErr: "link-local"},
		{name: "literal ipv6 loopback", url: "https://[::1]/repo.git", wantErr: "loopback"},

		{name: "dns to private", url: "https://evil.internal/repo.git", wantErr: "private"},
		{name: "dns to loopback", url: "https://loop.example/repo.git", wantErr: "loopback"},
		{name: "dns to metadata", url: "https://link.example/repo.git", wantErr: "link-local"},
		{name: "dns to cgnat", url: "https://cgnat.example/repo.git", wantErr: "CGNAT"},
		{name: "dns to ula", url: "https://ula.example/repo.git", wantErr: "private"},
		{name: "dns mixed public+private", url: "https://mixed.example/repo.git", wantErr: "private"},

		{
			name:      "hostname allowlist bypasses private",
			url:       "https://allowme.example/repo.git",
			allowlist: []string{"allowme.example"},
		},
		{
			name:      "cidr allowlist bypasses literal",
			url:       "https://10.9.9.9/repo.git",
			allowlist: []string{"10.9.9.0/24"},
		},
		{
			name:    "allowlist does not reopen http",
			url:     "http://github.com/org/repo.git",
			allowlist: []string{"github.com"},
			wantErr: "http is not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := base
			p.Allowlist = tt.allowlist
			err := p.Validate(tt.url)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParseAllowlist(t *testing.T) {
	got := parseAllowlist(" GitHub.com , 10.0.0.0/8,, ")
	if len(got) != 2 || got[0] != "github.com" || got[1] != "10.0.0.0/8" {
		t.Fatalf("got %#v", got)
	}
}

func TestExtractHost_SCP(t *testing.T) {
	host, err := extractHost("git@github.com:org/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if host != "github.com" {
		t.Fatalf("host=%q", host)
	}
}
