package version

import (
	_ "embed"
	"os"
	"strings"
)

//go:embed VERSION
var embedded string

// BuildVersion may be set at link time:
//
//	-ldflags "-X github.com/xiron902224/norix/control-plane/internal/version.BuildVersion=1.0.0-beta.3"
var BuildVersion string

// Current returns the running control-plane version.
// Order: NORIX_VERSION env → ldflags BuildVersion → embedded VERSION file.
func Current() string {
	if v := strings.TrimSpace(os.Getenv("NORIX_VERSION")); v != "" {
		return strings.TrimPrefix(v, "v")
	}
	if v := strings.TrimSpace(BuildVersion); v != "" {
		return strings.TrimPrefix(v, "v")
	}
	return strings.TrimSpace(embedded)
}
