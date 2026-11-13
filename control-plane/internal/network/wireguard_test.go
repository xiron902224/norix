package network

import (
	"strings"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	a, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if a.PublicKey == "" || a.PrivateKey == "" {
		t.Fatal("empty keys")
	}
	if a.PublicKey == b.PublicKey {
		t.Fatal("keys should differ")
	}
}

func TestNodeAddressAndFQDN(t *testing.T) {
	if got := NodeAddress(1); got != "10.88.0.1/32" {
		t.Fatalf("got %s", got)
	}
	if got := ServiceFQDN("Demo App", "Web_API"); got != "web-api.demo-app.norix.internal" {
		t.Fatalf("got %s", got)
	}
}

func TestRenderWGQuick(t *testing.T) {
	cfg := RenderWGQuick("wg-norix", "priv", "10.88.0.1/32", 51820, []PeerConfig{{
		PublicKey: "pub", AllowedIPs: "10.88.0.2/32", Endpoint: "1.2.3.4:51820",
	}})
	for _, want := range []string{"[Interface]", "PrivateKey = priv", "[Peer]", "Endpoint = 1.2.3.4:51820"} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("missing %q in %s", want, cfg)
		}
	}
}

func TestRenderNftables(t *testing.T) {
	out := RenderNftables([]ProjectIsolation{{
		ProjectName: "demo", Mark: 101, NodeAddrs: []string{"10.88.0.1/32"},
	}}, []EgressRule{{
		WorkloadName: "web", Mode: "allowlist", Allowlist: []string{"1.1.1.1/32"},
	}})
	if !strings.Contains(out, "norix-project-isolation") || !strings.Contains(out, "1.1.1.1/32") {
		t.Fatalf("unexpected ruleset: %s", out)
	}
}

func TestRenderHostsFileServiceDiscovery(t *testing.T) {
	out := RenderHostsFile([]DNSRecord{
		{Name: ServiceFQDN("blog", "web"), Type: "A", Value: "10.88.0.2"},
		{Name: "skip.me", Type: "CNAME", Value: "other"},
		{Name: "", Type: "A", Value: "10.88.0.3"},
	})
	if !strings.Contains(out, "10.88.0.2 web.blog.norix.internal") {
		t.Fatalf("missing A record: %s", out)
	}
	if strings.Contains(out, "skip.me") {
		t.Fatal("CNAME must be skipped in hosts snapshot")
	}
}

func TestCrossProjectIsolationRulePresent(t *testing.T) {
	out := RenderNftables([]ProjectIsolation{
		{ProjectName: "a", Mark: 101, NodeAddrs: []string{"10.88.0.1/32"}},
		{ProjectName: "b", Mark: 102, NodeAddrs: []string{"10.88.0.2/32"}},
	}, nil)
	if !strings.Contains(out, "meta mark != ct mark drop") {
		t.Fatal("expected cross-project drop rule")
	}
	if !strings.Contains(out, "ip saddr 10.88.0.1 meta mark set 101") {
		t.Fatal("expected project a mark")
	}
	if !strings.Contains(out, "ip saddr 10.88.0.2 meta mark set 102") {
		t.Fatal("expected project b mark")
	}
}

func TestIssueSelfSignedWildcard(t *testing.T) {
	bundle, err := IssueSelfSignedWildcard(WildcardHostname(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bundle.CertPEM, "BEGIN CERTIFICATE") {
		t.Fatal("missing cert pem")
	}
	if !strings.Contains(bundle.KeyPEM, "BEGIN EC PRIVATE KEY") {
		t.Fatal("missing key pem")
	}
}
