package secrets

import "testing"

func TestRequireConfiguredFailClosed(t *testing.T) {
	t.Setenv("NORIX_BUILD_SECRETS_KEY", "")
	t.Setenv(AllowInsecureDevEnv, "")
	if err := RequireConfigured(); err == nil {
		t.Fatal("expected error when secrets key missing")
	}
}

func TestRequireConfiguredAllowsInsecureDev(t *testing.T) {
	t.Setenv("NORIX_BUILD_SECRETS_KEY", "")
	t.Setenv(AllowInsecureDevEnv, "1")
	if err := RequireConfigured(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireConfiguredAcceptsValidKey(t *testing.T) {
	t.Setenv(AllowInsecureDevEnv, "")
	t.Setenv("NORIX_BUILD_SECRETS_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err := RequireConfigured(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireConfiguredRejectsShortKey(t *testing.T) {
	t.Setenv(AllowInsecureDevEnv, "")
	t.Setenv("NORIX_BUILD_SECRETS_KEY", "abcd")
	if err := RequireConfigured(); err == nil {
		t.Fatal("expected error for short key")
	}
}
