package wasm

import "testing"

func TestDefaultLimitsValidate(t *testing.T) {
	if err := DefaultLimits().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestServeWASIHTTPRequiresArtifact(t *testing.T) {
	_, _, err := ServeWASIHTTP(t.Context(), WASIHTTPConfig{
		ArtifactPath: "",
		Limits:       DefaultLimits(),
	})
	if err == nil {
		t.Fatal("expected error for empty artifact")
	}
}
