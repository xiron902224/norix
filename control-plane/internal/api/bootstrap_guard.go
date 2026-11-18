package api

import (
	"context"
	"net"
	"os"
	"strings"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
)

// AllowInsecureBootstrapEnv opts into first-admin bootstrap over plain HTTP
// from non-loopback clients (labs / Docker published ports before TLS).
const AllowInsecureBootstrapEnv = "NORIX_ALLOW_INSECURE_BOOTSTRAP"

const bootstrapInsecureHint = "first-admin bootstrap requires HTTPS or a loopback client " +
	"(ssh -L 8000:127.0.0.1:8000 user@host, then open http://127.0.0.1:8000). " +
	"For labs only set NORIX_ALLOW_INSECURE_BOOTSTRAP=1. " +
	"See docs/user/production-tls.md"

func insecureBootstrapAllowed() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(AllowInsecureBootstrapEnv)))
	return v == "1" || v == "true" || v == "yes"
}

// peerIP extracts the IP from RemoteAddr (host:port or bare IP).
func peerIP(remoteAddr string) net.IP {
	host := strings.TrimSpace(remoteAddr)
	if host == "" {
		return nil
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	// Strip IPv6 brackets if present without port.
	host = strings.Trim(host, "[]")
	return net.ParseIP(host)
}

func isLoopbackPeer(remoteAddr string) bool {
	ip := peerIP(remoteAddr)
	return ip != nil && ip.IsLoopback()
}

type headerGetter interface {
	Get(key string) string
}

// requestClaimsHTTPS is true for direct TLS or X-Forwarded-Proto from a loopback proxy.
func requestClaimsHTTPS(remoteAddr string, header headerGetter, tlsActive bool) bool {
	if tlsActive {
		return true
	}
	if !isLoopbackPeer(remoteAddr) {
		// Never trust X-Forwarded-Proto from a non-loopback peer (spoofable).
		return false
	}
	proto := header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = header.Get("X-Forwarded-Protocol")
	}
	// Take rightmost / last value if comma-separated.
	if i := strings.LastIndex(proto, ","); i >= 0 {
		proto = proto[i+1:]
	}
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

// BootstrapTransportAllowed reports whether CreateInitialAdmin may proceed.
func BootstrapTransportAllowed(remoteAddr string, header headerGetter, tlsActive bool) bool {
	if insecureBootstrapAllowed() {
		return true
	}
	if isLoopbackPeer(remoteAddr) {
		return true
	}
	return requestClaimsHTTPS(remoteAddr, header, tlsActive)
}

func requireSecureBootstrap(ctx context.Context) error {
	if insecureBootstrapAllowed() {
		return nil
	}
	callInfo, ok := connect.CallInfoForServerContext(ctx)
	if !ok {
		return connect.NewError(connect.CodeFailedPrecondition, bootstrapInsecureHint)
	}
	// Connect v2 puts the TCP peer on CallInfo.PeerAddr. TransportInfo is
	// *connecthttp.ServerInfo and does not expose RemoteAddr().
	remoteAddr := callInfo.PeerAddr
	tlsActive := false
	if info, ok := connecthttp.ServerInfoForContext(ctx); ok && info.TLS() != nil {
		tlsActive = true
	}
	if BootstrapTransportAllowed(remoteAddr, callInfo.RequestHeader(), tlsActive) {
		return nil
	}
	return connect.NewError(connect.CodeFailedPrecondition, bootstrapInsecureHint)
}
