package localruntime

import (
	"os"
	"strings"
)

// EmbeddedRuntimeEnv opts into reconcile-from-control-plane when docker.sock
// is mounted into the CP container. Default is off (#10): workloads run via agent.
const EmbeddedRuntimeEnv = "NORIX_EMBEDDED_RUNTIME"

// EmbeddedRuntimeEnabled reports whether the control plane should reconcile
// containers itself (requires a mounted Docker/nerdctl runtime).
func EmbeddedRuntimeEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(EmbeddedRuntimeEnv)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
