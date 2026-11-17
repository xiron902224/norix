package api

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
)

type clientCertFingerprintKey struct{}

// ClientCertFingerprintFromContext returns the SHA-256 hex of the TLS client cert DER.
func ClientCertFingerprintFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(clientCertFingerprintKey{}).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// WithClientCertFingerprint records the peer certificate fingerprint on the request context.
func WithClientCertFingerprint(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS != nil && len(request.TLS.PeerCertificates) > 0 {
			sum := sha256.Sum256(request.TLS.PeerCertificates[0].Raw)
			ctx := context.WithValue(request.Context(), clientCertFingerprintKey{}, hex.EncodeToString(sum[:]))
			request = request.WithContext(ctx)
		}
		next.ServeHTTP(response, request)
	})
}

// AgentMTLSRequired is true when NORIX_REQUIRE_AGENT_MTLS is set (#13).
func AgentMTLSRequired() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("NORIX_REQUIRE_AGENT_MTLS")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// LoadOptionalAgentMTLSConfig enables VerifyClientCertIfGiven when NORIX_AGENT_MTLS_CA is set.
// Combine with NORIX_TLS_CERT / NORIX_TLS_KEY for HTTPS on the control plane.
func LoadOptionalAgentMTLSConfig(base *tls.Config) (*tls.Config, error) {
	caPath := strings.TrimSpace(os.Getenv("NORIX_AGENT_MTLS_CA"))
	if caPath == "" {
		return base, nil
	}
	pem, err := os.ReadFile(caPath)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errString("NORIX_AGENT_MTLS_CA is not a valid PEM certificate pool")
	}
	cfg := base
	if cfg == nil {
		cfg = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		cfg = cfg.Clone()
	}
	cfg.ClientCAs = pool
	if AgentMTLSRequired() {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	} else {
		cfg.ClientAuth = tls.VerifyClientCertIfGiven
	}
	return cfg, nil
}

type errString string

func (e errString) Error() string { return string(e) }
