package wasm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// WASIHTTPConfig launches a wasmtime serve (WASI HTTP) worker for a .wasm artifact.
type WASIHTTPConfig struct {
	ArtifactPath string
	ListenAddr   string // e.g. 127.0.0.1:0 or :9090
	Limits       Limits
	WasmtimeBin  string
}

// ServeWASIHTTP starts `wasmtime serve` when wasmtime is available.
// Returns the command (already started) and the resolved listen address.
func ServeWASIHTTP(ctx context.Context, cfg WASIHTTPConfig) (*exec.Cmd, string, error) {
	if err := cfg.Limits.Validate(); err != nil {
		return nil, "", err
	}
	artifact := strings.TrimSpace(cfg.ArtifactPath)
	if artifact == "" {
		return nil, "", fmt.Errorf("artifact path required")
	}
	if _, err := os.Stat(artifact); err != nil {
		return nil, "", fmt.Errorf("artifact: %w", err)
	}
	bin := strings.TrimSpace(cfg.WasmtimeBin)
	if bin == "" {
		bin = "wasmtime"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return nil, "", fmt.Errorf("wasmtime not on PATH (required for WASI HTTP): %w", err)
	}
	addr := strings.TrimSpace(cfg.ListenAddr)
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	timeout := time.Duration(cfg.Limits.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		runCtx, cancel = context.WithTimeout(ctx, timeout+30*time.Second)
	}
	_ = cancel

	args := []string{
		"serve",
		"--addr", addr,
		"--fuel", strconv.FormatUint(cfg.Limits.Fuel, 10),
		"--max-memory-size", fmt.Sprintf("%dm", cfg.Limits.MemoryMB),
		artifact,
	}
	cmd := exec.CommandContext(runCtx, bin, args...)
	cmd.Dir = filepath.Dir(artifact)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("wasmtime serve: %w", err)
	}
	return cmd, addr, nil
}

// SmokeWASIHTTP checks that wasmtime supports the serve subcommand.
func SmokeWASIHTTP() error {
	bin, err := exec.LookPath("wasmtime")
	if err != nil {
		return fmt.Errorf("wasmtime missing: %w", err)
	}
	out, err := exec.Command(bin, "serve", "--help").CombinedOutput()
	if err != nil {
		return fmt.Errorf("wasmtime serve unavailable: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
