package localruntime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/xiron902224/norix/control-plane/internal/db"
)

// ApplyWasmPlan writes wasm_functions rows to a dry-run JSON file (S15 until agent.proto carries DesiredWasm).
func ApplyWasmPlan(ctx context.Context, orgID string, functions []db.WasmFunction) error {
	_ = ctx
	dir := os.Getenv("NORIX_WASM_STATE_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "norix-wasm")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	type row struct {
		Name          string `json:"name"`
		ImageRef      string `json:"image_ref"`
		FuelLimit     int64  `json:"fuel_limit"`
		MemoryLimitMB int32  `json:"memory_limit_mb"`
	}
	payload := make([]row, 0, len(functions))
	for _, fn := range functions {
		payload = append(payload, row{
			Name: fn.Name, ImageRef: fn.ImageRef, FuelLimit: fn.FuelLimit, MemoryLimitMB: fn.MemoryLimitMB,
		})
	}
	raw, err := json.MarshalIndent(map[string]any{
		"organization_id": orgID,
		"dry_run":         true,
		"functions":       payload,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "functions.json"), raw, 0o600)
}
