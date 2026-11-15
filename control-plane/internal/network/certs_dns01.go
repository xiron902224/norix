package network

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/digitalocean"
	"github.com/go-acme/lego/v4/providers/dns/route53"
	"github.com/go-acme/lego/v4/registration"
)

type acmeUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *acmeUser) GetEmail() string                        { return u.Email }
func (u acmeUser) GetRegistration() *registration.Resource { return u.Registration }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey       { return u.key }

// IssueACMEDNS01 obtains a certificate via lego DNS-01.
// Env:
//
//	NORIX_ACME_DNS_PROVIDER — cloudflare | route53 | digitalocean
//	NORIX_ACME_EMAIL — ACME account email
//	NORIX_ACME_DIRECTORY / NORIX_ACME_CA — optional ACME directory URL
//	Provider credentials via standard lego env (e.g. CLOUDFLARE_DNS_API_TOKEN).
func IssueACMEDNS01(hostname string) (CertificateBundle, error) {
	if hostname == "" {
		hostname = WildcardHostname()
	}
	providerName := strings.ToLower(strings.TrimSpace(os.Getenv("NORIX_ACME_DNS_PROVIDER")))
	email := strings.TrimSpace(firstNonEmpty(os.Getenv("NORIX_ACME_EMAIL"), os.Getenv("LEGO_EMAIL")))
	if providerName == "" || email == "" {
		return CertificateBundle{}, fmt.Errorf("NORIX_ACME_DNS_PROVIDER and NORIX_ACME_EMAIL required for DNS-01")
	}

	dnsProvider, err := newDNS01Provider(providerName)
	if err != nil {
		return CertificateBundle{}, err
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return CertificateBundle{}, err
	}
	user := &acmeUser{Email: email, key: privateKey}

	config := lego.NewConfig(user)
	config.Certificate.KeyType = certcrypto.EC256
	if dir := strings.TrimSpace(firstNonEmpty(os.Getenv("NORIX_ACME_DIRECTORY"), os.Getenv("NORIX_ACME_CA"))); dir != "" {
		config.CADirURL = dir
	}

	client, err := lego.NewClient(config)
	if err != nil {
		return CertificateBundle{}, err
	}

	opts := make([]dns01.ChallengeOption, 0, 1)
	if resolvers := strings.Fields(os.Getenv("NORIX_ACME_DNS_RESOLVERS")); len(resolvers) > 0 {
		opts = append(opts, dns01.AddRecursiveNameservers(resolvers))
	}
	if err := client.Challenge.SetDNS01Provider(dnsProvider, opts...); err != nil {
		return CertificateBundle{}, err
	}

	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return CertificateBundle{}, err
	}
	user.Registration = reg

	certificates, err := client.Certificate.Obtain(certificate.ObtainRequest{
		Domains: []string{hostname},
		Bundle:  true,
	})
	if err != nil {
		return CertificateBundle{}, err
	}

	notBefore, notAfter := parseCertWindow(certificates.Certificate)
	return CertificateBundle{
		Hostname:  hostname,
		CertPEM:   string(certificates.Certificate),
		KeyPEM:    string(certificates.PrivateKey),
		NotBefore: notBefore,
		NotAfter:  notAfter,
		Issuer:    "acme-dns01:" + providerName,
	}, nil
}

func newDNS01Provider(name string) (challenge.Provider, error) {
	switch name {
	case "cloudflare", "cf":
		return cloudflare.NewDNSProvider()
	case "route53", "aws":
		return route53.NewDNSProvider()
	case "digitalocean", "do":
		return digitalocean.NewDNSProvider()
	default:
		return nil, fmt.Errorf("unsupported NORIX_ACME_DNS_PROVIDER %q (want cloudflare|route53|digitalocean)", name)
	}
}

func parseCertWindow(certPEM []byte) (time.Time, time.Time) {
	now := time.Now().UTC()
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return now, now.Add(90 * 24 * time.Hour)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return now, now.Add(90 * 24 * time.Hour)
	}
	return cert.NotBefore.UTC(), cert.NotAfter.UTC()
}
