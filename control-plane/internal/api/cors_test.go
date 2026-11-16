package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCORSAllowsViteOrigin(t *testing.T) {
	handler := WithCORS(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodOptions, "/api/v1.AuthService/Login", nil)
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("Allow-Origin = %q", got)
	}
}

func TestWithCORSWildcardRequiresOptIn(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	t.Setenv("NORIX_CORS_ALLOW_WILDCARD", "")
	t.Setenv("NORIX_WEB_ORIGIN", "")
	t.Setenv("NORIX_PUBLIC_URL", "")

	handler := WithCORS(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodOptions, "/api/v1.AuthService/Login", nil)
	request.Header.Set("Origin", "https://evil.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("wildcard without opt-in must not allow, got %q", got)
	}

	t.Setenv("NORIX_CORS_ALLOW_WILDCARD", "1")
	handler = WithCORS(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
	}))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://evil.example" {
		t.Fatalf("Allow-Origin = %q after opt-in", got)
	}
}

func TestWithCORSIncludesPanelOrigin(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	t.Setenv("NORIX_CORS_ALLOW_WILDCARD", "")
	t.Setenv("NORIX_PUBLIC_URL", "https://norix.example.com")
	t.Setenv("NORIX_WEB_ORIGIN", "https://norix.example.com")

	handler := WithCORS(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodOptions, "/api/v1.AuthService/Login", nil)
	request.Header.Set("Origin", "https://norix.example.com")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://norix.example.com" {
		t.Fatalf("Allow-Origin = %q", got)
	}
}
