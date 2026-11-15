package localruntime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/xiron902224/norix/control-plane/internal/db"
)

// ApplyWasmPlan materializes the desired wasm plan for the embedded local runtime
// (same on-disk contract the remote agent writes under workdir/wasm/).
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
		ID            string `json:"id"`
		Name          string `json:"name"`
		ImageRef      string `json:"image_ref"`
		FuelLimit     int64  `json:"fuel_limit"`
		MemoryLimitMB int32  `json:"memory_limit_mb"`
		Status        string `json:"status"`
	}
	payload := make([]row, 0, len(functions))
	for _, fn := range functions {
		status := fn.Status
		if status == "" {
			status = "ready"
		}
		payload = append(payload, row{
			ID: fn.ID, Name: fn.Name, ImageRef: fn.ImageRef,
			FuelLimit: fn.FuelLimit, MemoryLimitMB: fn.MemoryLimitMB, Status: status,
		})
	}
	raw, err := json.MarshalIndent(map[string]any{
		"organization_id": orgID,
		"dry_run":         false,
		"functions":       payload,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "functions.json"), raw, 0o600)
}
