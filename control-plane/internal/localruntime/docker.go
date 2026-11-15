package localruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

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
		if !isContainerRuntime(item.Runtime) {
			observed = append(observed, db.ObservedWorkload{
				ID: item.ID, Status: "failed", ObservedGeneration: item.DesiredGeneration,
				HostPort: item.HostPort,
				Message:  fmt.Sprintf("runtime %q is not reconciled by the container engine; use agent wasm/microvm paths", item.Runtime),
				LogLines: []string{fmt.Sprintf("skipped non-container runtime=%s", item.Runtime)},
			})
			continue
		}
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

	ensureOnly := desired.Action == "none" || desired.Action == ""

	if existing, ok := runtime.inspectRunning(ctx, name); ok {
		generation := labelInt64(existing, "norix.generation")
		image := stringField(existing, "Config", "Image")
		if generation == desired.DesiredGeneration && imageMatches(image, desired.Image) {
			if ensureOnly && desired.HostPort > 0 && !tcpOpen(ctx, desired.HostPort) {
				logs, _ := runtime.logs(ctx, name, 40)
				_ = runtime.remove(ctx, name)
				return db.ObservedWorkload{
					ID:                 desired.ID,
					Status:             "failed",
					ContainerID:        stringField(existing, "Id"),
					ObservedGeneration: desired.DesiredGeneration,
					HostPort:           desired.HostPort,
					Message:            fmt.Sprintf("host port :%d not accepting connections", desired.HostPort),
					LogLines:           logs,
				}, nil
			}
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
	} else if info, exists := runtime.inspectAny(ctx, name); exists {
		// Container present but not healthy. For ensure-only ticks, surface
		// failure instead of rm/recreate thrashing (which resets RestartCount
		// and keeps the UI stuck on "running").
		if ensureOnly {
			logs, _ := runtime.logs(ctx, name, 40)
			msg := unhealthyMessage(info)
			_ = runtime.remove(ctx, name)
			return db.ObservedWorkload{
				ID:                 desired.ID,
				Status:             "failed",
				ContainerID:        stringField(info, "Id"),
				ObservedGeneration: desired.DesiredGeneration,
				HostPort:           desired.HostPort,
				Message:            msg,
				LogLines:           logs,
			}, nil
		}
		_ = runtime.remove(ctx, name)
	} else if !ensureOnly {
		// Explicit run: clear any name conflict leftovers.
		_ = runtime.remove(ctx, name)
	}
	// ensureOnly + missing container → fall through and recreate (self-heal).

	if err := runtime.pull(ctx, desired.Image); err != nil {
		return db.ObservedWorkload{}, err
	}
	containerID, err := runtime.run(ctx, desired, name)
	if err != nil {
		return db.ObservedWorkload{}, err
	}

	// Give the process a moment; crash-loops often exit within 1–2s.
	select {
	case <-ctx.Done():
		return db.ObservedWorkload{}, ctx.Err()
	case <-time.After(2 * time.Second):
	}

	if settled, ok := runtime.inspectRunning(ctx, name); ok {
		if desired.HostPort > 0 && !tcpOpen(ctx, desired.HostPort) {
			logs, _ := runtime.logs(ctx, name, 40)
			return db.ObservedWorkload{
				ID:                 desired.ID,
				Status:             "failed",
				ContainerID:        stringField(settled, "Id"),
				ObservedGeneration: desired.DesiredGeneration,
				HostPort:           desired.HostPort,
				Message:            fmt.Sprintf("host port :%d not accepting connections", desired.HostPort),
				LogLines:           logs,
			}, nil
		}
		logs, _ := runtime.logs(ctx, name, 40)
		return db.ObservedWorkload{
			ID:                 desired.ID,
			Status:             "running",
			ContainerID:        stringField(settled, "Id"),
			ObservedGeneration: desired.DesiredGeneration,
			HostPort:           desired.HostPort,
			LogLines:           logs,
		}, nil
	}

	logs, _ := runtime.logs(ctx, name, 40)
	info, _ := runtime.inspectAny(ctx, name)
	msg := "container exited or is restarting after start"
	if info != nil {
		msg = unhealthyMessage(info)
	}
	return db.ObservedWorkload{
		ID:                 desired.ID,
		Status:             "failed",
		ContainerID:        containerID,
		ObservedGeneration: desired.DesiredGeneration,
		HostPort:           desired.HostPort,
		Message:            msg,
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

func (runtime *Runtime) ensureNetwork(ctx context.Context, name string) error {
	if commandOK(ctx, runtime.bin(), "network", "inspect", name) {
		return nil
	}
	cmd := exec.CommandContext(ctx, runtime.bin(), "network", "create", name)
	if err := cmd.Run(); err != nil {
		if commandOK(ctx, runtime.bin(), "network", "inspect", name) {
			return nil
		}
		return fmt.Errorf("network create %s: %w", name, err)
	}
	return nil
}

func isContainerRuntime(runtime string) bool {
	switch strings.ToLower(strings.TrimSpace(runtime)) {
	case "", "container", "docker-compose", "static-site", "runtime_type_container", "1", "2", "5":
		return true
	default:
		return false
	}
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
	if desired.CPUShares > 0 {
		args = append(args, "--cpu-shares", fmt.Sprintf("%d", desired.CPUShares))
	}
	if name := strings.TrimSpace(desired.NetworkName); name != "" {
		_ = runtime.ensureNetwork(ctx, name)
		args = append(args, "--network", name)
		if alias := strings.TrimSpace(desired.NetworkAlias); alias != "" {
			args = append(args, "--network-alias", alias)
		}
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

func (runtime *Runtime) inspectAny(ctx context.Context, name string) (map[string]any, bool) {
	cmd := exec.CommandContext(ctx, runtime.bin(), "inspect", name)
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	var parsed []map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil || len(parsed) == 0 {
		return nil, false
	}
	return parsed[0], true
}

func (runtime *Runtime) inspectRunning(ctx context.Context, name string) (map[string]any, bool) {
	info, ok := runtime.inspectAny(ctx, name)
	if !ok {
		return nil, false
	}
	state, _ := info["State"].(map[string]any)
	running, _ := state["Running"].(bool)
	restarting, _ := state["Restarting"].(bool)
	oomKilled, _ := state["OOMKilled"].(bool)
	// Docker reports Running=true while Restarting=true during crash loops;
	// treat those as not healthy so reconcile surfaces failure.
	if !running || restarting || oomKilled {
		return nil, false
	}
	return info, true
}

func tcpOpen(ctx context.Context, port int32) bool {
	if port <= 0 {
		return true
	}
	dialer := net.Dialer{Timeout: 800 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func unhealthyMessage(info map[string]any) string {
	state, _ := info["State"].(map[string]any)
	if state == nil {
		return "container unhealthy"
	}
	status, _ := state["Status"].(string)
	errMsg, _ := state["Error"].(string)
	exitCode, _ := state["ExitCode"].(float64)
	restarting, _ := state["Restarting"].(bool)
	oomKilled, _ := state["OOMKilled"].(bool)
	restartCount, _ := state["RestartCount"].(float64)
	parts := []string{}
	if status != "" {
		parts = append(parts, "status="+status)
	}
	if restarting {
		parts = append(parts, "restarting")
	}
	if oomKilled {
		parts = append(parts, "oom-killed")
	}
	if restartCount > 0 {
		parts = append(parts, fmt.Sprintf("restarts=%.0f", restartCount))
	}
	if exitCode != 0 {
		parts = append(parts, fmt.Sprintf("exit=%.0f", exitCode))
	}
	if errMsg != "" {
		parts = append(parts, errMsg)
	}
	if len(parts) == 0 {
		return "container unhealthy"
	}
	return "container unhealthy: " + strings.Join(parts, " ")
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
