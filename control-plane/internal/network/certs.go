package network

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"
)

type CertificateBundle struct {
	Hostname  string
	CertPEM   string
	KeyPEM    string
	NotBefore time.Time
	NotAfter  time.Time
	Issuer    string
}

// IssuePlatformCertificate prefers DNS-01 ACME (lego) when configured; otherwise self-signed.
func IssuePlatformCertificate(hostname string, validFor time.Duration) (CertificateBundle, error) {
	if DNS01Configured() {
		bundle, err := IssueACMEDNS01(hostname)
		if err == nil {
			return bundle, nil
		}
		// Fall through to self-signed so edge keeps working when ACME/DNS is misconfigured.
	}
	return IssueSelfSignedWildcard(hostname, validFor)
}

// DNS01Configured reports whether DNS-01 ACME env is present.
func DNS01Configured() bool {
	provider := strings.TrimSpace(os.Getenv("NORIX_ACME_DNS_PROVIDER"))
	email := strings.TrimSpace(firstNonEmpty(os.Getenv("NORIX_ACME_EMAIL"), os.Getenv("LEGO_EMAIL")))
	return provider != "" && email != ""
}

// IssueSelfSignedWildcard issues a platform wildcard cert (fallback when DNS-01 is off).
func IssueSelfSignedWildcard(hostname string, validFor time.Duration) (CertificateBundle, error) {
	if hostname == "" {
		hostname = "*." + DNSSuffix
	}
	if validFor <= 0 {
		validFor = 90 * 24 * time.Hour
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return CertificateBundle{}, err
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return CertificateBundle{}, err
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Norix"},
			CommonName:   hostname,
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(validFor),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{hostname, DNSSuffix},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return CertificateBundle{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return CertificateBundle{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return CertificateBundle{
		Hostname:  hostname,
		CertPEM:   string(certPEM),
		KeyPEM:    string(keyPEM),
		NotBefore: template.NotBefore,
		NotAfter:  template.NotAfter,
		Issuer:    "norix-internal",
	}, nil
}

func WildcardHostname() string {
	if host := strings.TrimSpace(os.Getenv("NORIX_PLATFORM_HOSTNAME")); host != "" {
		if strings.HasPrefix(host, "*.") {
			return host
		}
		return "*." + host
	}
	return fmt.Sprintf("*.%s", DNSSuffix)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
