package network

import "testing"

func TestDNS01Configured(t *testing.T) {
	t.Setenv("NORIX_ACME_DNS_PROVIDER", "")
	t.Setenv("NORIX_ACME_EMAIL", "")
	if DNS01Configured() {
		t.Fatal("expected false when unset")
	}
	t.Setenv("NORIX_ACME_DNS_PROVIDER", "cloudflare")
	t.Setenv("NORIX_ACME_EMAIL", "ops@example.com")
	if !DNS01Configured() {
		t.Fatal("expected true when provider+email set")
	}
}

func TestIssuePlatformCertificateFallsBackToSelfSigned(t *testing.T) {
	t.Setenv("NORIX_ACME_DNS_PROVIDER", "")
	t.Setenv("NORIX_ACME_EMAIL", "")
	bundle, err := IssuePlatformCertificate(WildcardHostname(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Issuer != "norix-internal" {
		t.Fatalf("issuer = %q", bundle.Issuer)
	}
}

func TestNewDNS01ProviderUnsupported(t *testing.T) {
	if _, err := newDNS01Provider("not-a-provider"); err == nil {
		t.Fatal("expected error")
	}
}
