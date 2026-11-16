package secrets

import (
	"fmt"
	"os"
	"strings"
)

// AllowInsecureDevEnv is the opt-in that permits starting without
// NORIX_BUILD_SECRETS_KEY (plaintext TOTP / WireGuard storage).
const AllowInsecureDevEnv = "NORIX_ALLOW_INSECURE_DEV_SECRETS"

// InsecureDevAllowed reports whether plaintext secret storage is explicitly opted in.
func InsecureDevAllowed() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(AllowInsecureDevEnv)))
	return v == "1" || v == "true" || v == "yes"
}

// RequireConfigured fails closed unless a valid NORIX_BUILD_SECRETS_KEY is set,
// or NORIX_ALLOW_INSECURE_DEV_SECRETS explicitly opts into the weak/plaintext path.
func RequireConfigured() error {
	if _, err := KeyFromEnv(); err == nil {
		return nil
	} else if InsecureDevAllowed() {
		return nil
	}
	return fmt.Errorf(
		"NORIX_BUILD_SECRETS_KEY is required (64 hex chars); set it or opt in with %s=1 for local/dev only",
		AllowInsecureDevEnv,
	)
}
