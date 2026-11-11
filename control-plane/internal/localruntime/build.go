package localruntime

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/xiron902224/norix/control-plane/internal/db"
)

func (runtime *Runtime) RunBuilds(ctx context.Context, builds []db.DesiredBuild) []db.ObservedBuild {
	observed := make([]db.ObservedBuild, 0, len(builds))
	for _, build := range builds {
		result, err := runtime.runBuild(ctx, build)
		if err != nil {
			observed = append(observed, db.ObservedBuild{
				ID:       build.ID,
				Status:   "failed",
				Message:  err.Error(),
				LogLines: []string{err.Error()},
			})
			continue
		}
		observed = append(observed, result)
	}
	return observed
}

func (runtime *Runtime) runBuild(ctx context.Context, build db.DesiredBuild) (db.ObservedBuild, error) {
	logs := []string{fmt.Sprintf("building %s @ %s", build.GitURL, build.GitRef)}
	workdir, err := os.MkdirTemp("", "norix-build-*")
	if err != nil {
		return db.ObservedBuild{}, err
	}
	defer os.RemoveAll(workdir)

	cloneDir := filepath.Join(workdir, "src")
	ref := build.GitRef
	if ref == "" {
		ref = "main"
	}
	clone := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", ref, build.GitURL, cloneDir)
	cloneOut, cloneErr := clone.CombinedOutput()
	logs = append(logs, truncateLines(string(cloneOut), 40)...)
	if cloneErr != nil {
		// fallback without --branch for default HEAD
		_ = os.RemoveAll(cloneDir)
		clone = exec.CommandContext(ctx, "git", "clone", "--depth", "1", build.GitURL, cloneDir)
		cloneOut, cloneErr = clone.CombinedOutput()
		logs = append(logs, truncateLines(string(cloneOut), 40)...)
		if cloneErr != nil {
			return db.ObservedBuild{}, fmt.Errorf("git clone: %w", cloneErr)
		}
	}

	dockerfile := build.DockerfilePath
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	dockerfilePath := filepath.Join(cloneDir, dockerfile)
	if _, err := os.Stat(dockerfilePath); err != nil {
		return db.ObservedBuild{}, fmt.Errorf("dockerfile not found at %s", dockerfile)
	}

	args := []string{"build", "-f", dockerfile, "-t", build.ImageTag}
	for key, value := range build.BuildArgs {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, value))
	}
	args = append(args, ".")
	cmd := exec.CommandContext(ctx, string(runtime.engine), args...)
	cmd.Dir = cloneDir
	buildOut, buildErr := cmd.CombinedOutput()
	logs = append(logs, truncateLines(string(buildOut), 80)...)
	if buildErr != nil {
		return db.ObservedBuild{ID: build.ID, Status: "failed", Message: buildErr.Error(), LogLines: logs}, buildErr
	}

	image := build.ImageTag
	if build.Registry != "" && strings.Contains(build.ImageTag, build.Registry) {
		logs = append(logs, fmt.Sprintf("pushing %s", build.ImageTag))
		push := exec.CommandContext(ctx, string(runtime.engine), "push", build.ImageTag)
		pushOut, pushErr := push.CombinedOutput()
		logs = append(logs, truncateLines(string(pushOut), 40)...)
		if pushErr != nil {
			logs = append(logs, fmt.Sprintf("push failed (%v); using local image tag", pushErr))
		}
	}
	logs = append(logs, "build succeeded")
	return db.ObservedBuild{
		ID:       build.ID,
		Status:   "succeeded",
		Image:    image,
		LogLines: logs,
	}, nil
}

func truncateLines(output string, max int) []string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	lines := make([]string, 0, max)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) >= max {
			break
		}
	}
	return lines
}
