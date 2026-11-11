package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiron902224/norix/control-plane/internal/api"
)

func TestHealthEndpoint(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	newHandler(api.AuthService{}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Body.String() != "ok\n" {
		t.Errorf("health body = %q, want %q", response.Body.String(), "ok\n")
	}
}
