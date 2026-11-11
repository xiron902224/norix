package localruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/xiron902224/norix/control-plane/internal/db"
)

type Engine string

const (
	EngineDocker  Engine = "docker"
	EngineNerdctl Engine = "nerdctl"
)

type Runtime struct {
	engine Engine
}

func Detect(ctx context.Context) (*Runtime, error) {
	if commandOK(ctx, "docker", "info") {
		return &Runtime{engine: EngineDocker}, nil
	}
	if commandOK(ctx, "nerdctl", "info") {
		return &Runtime{engine: EngineNerdctl}, nil
	}
	return nil, fmt.Errorf("neither docker nor nerdctl is available")
}

func (runtime *Runtime) Reconcile(ctx context.Context, desired []db.DesiredWorkload) []db.ObservedWorkload {
	observed := make([]db.ObservedWorkload, 0, len(desired))
	for _, item := range desired {
		result, err := runtime.reconcileOne(ctx, item)
		if err != nil {
			observed = append(observed, db.ObservedWorkload{
				ID:                 item.ID,
				Status:             "failed",
				ObservedGeneration: item.DesiredGeneration,
				Message:            err.Error(),
				HostPort:           item.HostPort,
				LogLines:           []string{fmt.Sprintf("reconcile error: %s", err.Error())},
			})
			continue
		}
		observed = append(observed, result)
	}
	return observed
}

func (runtime *Runtime) reconcileOne(ctx context.Context, desired db.DesiredWorkload) (db.ObservedWorkload, error) {
	name := containerName(desired.ID)
	if desired.Action == "stop" {
		_ = runtime.remove(ctx, name)
		return db.ObservedWorkload{
			ID:                 desired.ID,
			Status:             "stopped",
			ObservedGeneration: desired.DesiredGeneration,
			HostPort:           desired.HostPort,
			LogLines:           []string{fmt.Sprintf("stopped %s", name)},
		}, nil
	}

	if existing, ok := runtime.inspectRunning(ctx, name); ok {
		generation := labelInt64(existing, "norix.generation")
		image := stringField(existing, "Config", "Image")
		if generation == desired.DesiredGeneration && imageMatches(image, desired.Image) {
			logs, _ := runtime.logs(ctx, name, 40)
			return db.ObservedWorkload{
				ID:                 desired.ID,
				Status:             "running",
				ContainerID:        stringField(existing, "Id"),
				ObservedGeneration: desired.DesiredGeneration,
				HostPort:           desired.HostPort,
				LogLines:           logs,
			}, nil
		}
		_ = runtime.remove(ctx, name)
	}

	if err := runtime.pull(ctx, desired.Image); err != nil {
		return db.ObservedWorkload{}, err
	}
	containerID, err := runtime.run(ctx, desired, name)
	if err != nil {
		return db.ObservedWorkload{}, err
	}
	logs, _ := runtime.logs(ctx, name, 40)
	return db.ObservedWorkload{
		ID:                 desired.ID,
		Status:             "running",
		ContainerID:        containerID,
		ObservedGeneration: desired.DesiredGeneration,
		HostPort:           desired.HostPort,
		LogLines:           logs,
	}, nil
}

func (runtime *Runtime) bin() string {
	return string(runtime.engine)
}

func (runtime *Runtime) imageExists(ctx context.Context, image string) bool {
	cmd := exec.CommandContext(ctx, runtime.bin(), "image", "inspect", image)
	return cmd.Run() == nil
}

func (runtime *Runtime) pull(ctx context.Context, image string) error {
	// Local builds tag images on the node; don't require a reachable registry.
	if runtime.imageExists(ctx, image) {
		return nil
	}
	cmd := exec.CommandContext(ctx, runtime.bin(), "pull", image)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pull %s: %w (%s)", image, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (runtime *Runtime) run(ctx context.Context, desired db.DesiredWorkload, name string) (string, error) {
	port := fmt.Sprintf("%d:%d", desired.HostPort, max32(desired.ContainerPort, 1))
	memory := fmt.Sprintf("%dm", max64(desired.MemoryLimitMB, 64))
	args := []string{
		"run", "-d", "--name", name, "--restart", "unless-stopped",
		"-p", port, "--memory", memory,
		"--label", fmt.Sprintf("norix.workload_id=%s", desired.ID),
		"--label", fmt.Sprintf("norix.generation=%d", desired.DesiredGeneration),
		"--label", fmt.Sprintf("norix.project_id=%s", desired.ProjectID),
	}
	for key, value := range desired.Environment {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, value))
	}
	args = append(args, desired.Image)

	cmd := exec.CommandContext(ctx, runtime.bin(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("run failed: %s", strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (runtime *Runtime) remove(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, runtime.bin(), "rm", "-f", name)
	_ = cmd.Run()
	return nil
}

func (runtime *Runtime) inspectRunning(ctx context.Context, name string) (map[string]any, bool) {
	cmd := exec.CommandContext(ctx, runtime.bin(), "inspect", name)
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	var parsed []map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil || len(parsed) == 0 {
		return nil, false
	}
	state, _ := parsed[0]["State"].(map[string]any)
	running, _ := state["Running"].(bool)
	if !running {
		return nil, false
	}
	return parsed[0], true
}

func (runtime *Runtime) logs(ctx context.Context, name string, tail int) ([]string, error) {
	cmd := exec.CommandContext(ctx, runtime.bin(), "logs", "--tail", fmt.Sprintf("%d", tail), name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	lines := make([]string, 0)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func commandOK(ctx context.Context, bin string, args ...string) bool {
	cmd := exec.CommandContext(ctx, bin, args...)
	return cmd.Run() == nil
}

func containerName(workloadID string) string {
	compact := strings.Builder{}
	for _, ch := range workloadID {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			compact.WriteRune(ch)
		}
	}
	id := compact.String()
	if len(id) > 12 {
		id = id[:12]
	}
	return "norix-" + id
}

func imageMatches(running, desired string) bool {
	return running == desired || strings.HasSuffix(running, "/"+desired) || strings.Contains(running, desired)
}

func labelInt64(inspect map[string]any, key string) int64 {
	cfg, _ := inspect["Config"].(map[string]any)
	labels, _ := cfg["Labels"].(map[string]any)
	raw, _ := labels[key].(string)
	var value int64
	_, _ = fmt.Sscanf(raw, "%d", &value)
	return value
}

func stringField(inspect map[string]any, path ...string) string {
	var current any = inspect
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[key]
	}
	value, _ := current.(string)
	return value
}

func max32(value, floor int32) int32 {
	if value < floor {
		return floor
	}
	return value
}

func max64(value, floor int64) int64 {
	if value < floor {
		return floor
	}
	return value
}
