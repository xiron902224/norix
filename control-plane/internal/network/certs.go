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

// IssueSelfSignedWildcard issues a platform wildcard cert (DNS-01 path lands later with lego).
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
	return fmt.Sprintf("*.%s", DNSSuffix)
}
