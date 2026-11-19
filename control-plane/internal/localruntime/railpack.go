package localruntime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xiron902224/norix/control-plane/internal/db"
)

// detectRailpackSource returns a railpack build when no Dockerfile/compose exists
// but a known language/static marker is present (#18).
func detectRailpackSource(cloneDir string) (kind, reason string, ok bool) {
	markers := []struct {
		file  string
		label string
	}{
		{"package.json", "Node.js (package.json)"},
		{"go.mod", "Go (go.mod)"},
		{"pyproject.toml", "Python (pyproject.toml)"},
		{"requirements.txt", "Python (requirements.txt)"},
		{"Pipfile", "Python (Pipfile)"},
		{"index.html", "static site (index.html)"},
	}
	for _, m := range markers {
		if fileExists(filepath.Join(cloneDir, m.file)) {
			return "railpack", m.label, true
		}
	}
	return "", "", false
}

var buildKitMu sync.Mutex

func ensureBuildKitHost(ctx context.Context, appendLog func(string)) (string, error) {
	if host := strings.TrimSpace(os.Getenv("BUILDKIT_HOST")); host != "" {
		appendLog(fmt.Sprintf("using BUILDKIT_HOST=%s", host))
		return host, nil
	}
	// Parallel builds share one BuildKit container. Inspect-then-run must not race (#24).
	buildKitMu.Lock()
	defer buildKitMu.Unlock()
	const name = "norix-buildkit"
	inspect := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", name)
	out, err := inspect.CombinedOutput()
	running := err == nil && strings.TrimSpace(string(out)) == "true"
	if !running {
		appendLog("starting BuildKit container " + name)
		_ = exec.CommandContext(ctx, "docker", "rm", "-f", name).Run()
		// Optional limits for the shared daemon: NORIX_BUILDKIT_CPUS, NORIX_BUILDKIT_MEMORY.
		runArgs := []string{"run", "--rm", "--privileged", "-d"}
		if v := strings.TrimSpace(os.Getenv("NORIX_BUILDKIT_CPUS")); v != "" {
			runArgs = append(runArgs, "--cpus", v)
		}
		if v := strings.TrimSpace(os.Getenv("NORIX_BUILDKIT_MEMORY")); v != "" {
			runArgs = append(runArgs, "--memory", v)
		}
		runArgs = append(runArgs, "--name", name, "moby/buildkit:buildx-stable-1")
		start := exec.CommandContext(ctx, "docker", runArgs...)
		if startOut, startErr := start.CombinedOutput(); startErr != nil {
			return "", fmt.Errorf("start buildkit: %w (%s)", startErr, strings.TrimSpace(string(startOut)))
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	} else {
		appendLog("reusing BuildKit container " + name)
	}
	return "docker-container://" + name, nil
}

func (runtime *Runtime) buildRailpack(ctx context.Context, cloneDir string, build db.DesiredBuild, appendLog func(string)) error {
	_ = runtime
	if _, err := exec.LookPath("railpack"); err != nil {
		return fmt.Errorf("railpack not found in PATH — install from https://railpack.com/installation/ (curl -sSL https://railpack.com/install.sh | sh)")
	}
	host, err := ensureBuildKitHost(ctx, appendLog)
	if err != nil {
		return err
	}
	appendLog(fmt.Sprintf("railpack build --name %s .", build.ImageTag))
	cmd := exec.CommandContext(ctx, "railpack", "build",
		"--name", build.ImageTag,
		"--progress", "plain",
		".",
	)
	cmd.Dir = cloneDir
	cmd.Env = append(os.Environ(),
		"BUILDKIT_HOST="+host,
		"DOCKER_BUILDKIT=1",
	)
	return streamCommand(ctx, cmd, appendLog, "railpack build failed")
}
