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
		_ = os.RemoveAll(cloneDir)
		clone = exec.CommandContext(ctx, "git", "clone", "--depth", "1", build.GitURL, cloneDir)
		cloneOut, cloneErr = clone.CombinedOutput()
		logs = append(logs, truncateLines(string(cloneOut), 40)...)
		if cloneErr != nil {
			return db.ObservedBuild{}, fmt.Errorf("git clone: %w", cloneErr)
		}
	}

	kind, path, discoverLogs := resolveBuildSource(cloneDir, build.DockerfilePath)
	logs = append(logs, discoverLogs...)
	if kind == "" {
		return db.ObservedBuild{ID: build.ID, Status: "failed", Message: "no Dockerfile or compose file found", LogLines: logs}, fmt.Errorf("no Dockerfile or compose file found")
	}

	var buildErr error
	switch kind {
	case "dockerfile":
		logs = append(logs, fmt.Sprintf("using Dockerfile %s", path))
		buildErr = runtime.buildDockerfile(ctx, cloneDir, path, build, &logs)
	case "compose":
		logs = append(logs, fmt.Sprintf("using compose file %s", path))
		buildErr = runtime.buildCompose(ctx, cloneDir, path, build, &logs)
	default:
		buildErr = fmt.Errorf("unknown build kind %s", kind)
	}
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

func (runtime *Runtime) buildDockerfile(ctx context.Context, cloneDir, dockerfile string, build db.DesiredBuild, logs *[]string) error {
	args := []string{"build", "-f", dockerfile, "-t", build.ImageTag}
	for key, value := range build.BuildArgs {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, value))
	}
	args = append(args, ".")
	cmd := exec.CommandContext(ctx, string(runtime.engine), args...)
	cmd.Dir = cloneDir
	buildOut, buildErr := cmd.CombinedOutput()
	*logs = append(*logs, truncateLines(string(buildOut), 80)...)
	if buildErr != nil {
		return fmt.Errorf("docker build failed: %w", buildErr)
	}
	return nil
}

func (runtime *Runtime) buildCompose(ctx context.Context, cloneDir, composeFile string, build db.DesiredBuild, logs *[]string) error {
	project := "norix" + sanitizeProject(build.ID)
	bin := string(runtime.engine)
	// Prefer docker compose plugin; fall back to docker-compose binary.
	composeArgs := func(args ...string) *exec.Cmd {
		all := append([]string{"compose", "-p", project, "-f", composeFile}, args...)
		cmd := exec.CommandContext(ctx, bin, all...)
		cmd.Dir = cloneDir
		return cmd
	}

	servicesOut, err := composeArgs("config", "--services").CombinedOutput()
	*logs = append(*logs, truncateLines(string(servicesOut), 20)...)
	if err != nil {
		// try legacy docker-compose
		legacy := exec.CommandContext(ctx, "docker-compose", "-p", project, "-f", composeFile, "config", "--services")
		legacy.Dir = cloneDir
		servicesOut, err = legacy.CombinedOutput()
		*logs = append(*logs, truncateLines(string(servicesOut), 20)...)
		if err != nil {
			return fmt.Errorf("compose config failed: %w", err)
		}
		composeArgs = func(args ...string) *exec.Cmd {
			all := append([]string{"-p", project, "-f", composeFile}, args...)
			cmd := exec.CommandContext(ctx, "docker-compose", all...)
			cmd.Dir = cloneDir
			return cmd
		}
	}

	services := strings.Fields(string(servicesOut))
	if len(services) == 0 {
		return fmt.Errorf("compose file has no services")
	}
	service := pickComposeService(services)
	*logs = append(*logs, fmt.Sprintf("building compose service %s", service))

	buildCmd := composeArgs("build", service)
	buildOut, buildErr := buildCmd.CombinedOutput()
	*logs = append(*logs, truncateLines(string(buildOut), 80)...)
	if buildErr != nil {
		// Some compose files only have image: (prebuilt). Try pulling via up --no-start then tag.
		*logs = append(*logs, "compose build failed; trying pull/up for image-based services")
		upCmd := composeArgs("pull", service)
		upOut, _ := upCmd.CombinedOutput()
		*logs = append(*logs, truncateLines(string(upOut), 40)...)
	}

	localImage := project + "-" + service
	tagCmd := exec.CommandContext(ctx, bin, "tag", localImage, build.ImageTag)
	tagOut, tagErr := tagCmd.CombinedOutput()
	*logs = append(*logs, truncateLines(string(tagOut), 10)...)
	if tagErr != nil {
		// Compose v1 style underscore naming
		alt := project + "_" + service
		tagCmd = exec.CommandContext(ctx, bin, "tag", alt, build.ImageTag)
		tagOut, tagErr = tagCmd.CombinedOutput()
		*logs = append(*logs, truncateLines(string(tagOut), 10)...)
		if tagErr != nil {
			// Last resort: read image from compose config
			imgOut, imgErr := composeArgs("config", "--images", service).CombinedOutput()
			*logs = append(*logs, truncateLines(string(imgOut), 10)...)
			if imgErr != nil {
				return fmt.Errorf("compose image tag failed for %s / %s", localImage, alt)
			}
			images := strings.Fields(string(imgOut))
			if len(images) == 0 {
				return fmt.Errorf("compose produced no image for service %s", service)
			}
			tagCmd = exec.CommandContext(ctx, bin, "tag", images[0], build.ImageTag)
			tagOut, tagErr = tagCmd.CombinedOutput()
			*logs = append(*logs, truncateLines(string(tagOut), 10)...)
			if tagErr != nil {
				return fmt.Errorf("tag %s -> %s failed: %w", images[0], build.ImageTag, tagErr)
			}
		}
	}
	return nil
}

func resolveBuildSource(cloneDir, hint string) (kind, path string, logs []string) {
	hint = strings.TrimSpace(hint)
	if hint != "" {
		candidate := filepath.Join(cloneDir, hint)
		if fileExists(candidate) {
			if isComposeName(hint) {
				return "compose", hint, []string{fmt.Sprintf("hint %s is a compose file", hint)}
			}
			return "dockerfile", hint, []string{fmt.Sprintf("using hinted Dockerfile %s", hint)}
		}
		logs = append(logs, fmt.Sprintf("hinted path %s not found; auto-detecting", hint))
	}

	for _, name := range []string{"Dockerfile", "dockerfile", "Containerfile", "docker/Dockerfile", "build/Dockerfile", ".docker/Dockerfile"} {
		if fileExists(filepath.Join(cloneDir, name)) {
			return "dockerfile", name, append(logs, fmt.Sprintf("auto-detected Dockerfile %s", name))
		}
	}
	// shallow search (max depth 2)
	var foundDockerfile string
	_ = filepath.WalkDir(cloneDir, func(walkPath string, d os.DirEntry, err error) error {
		if err != nil || foundDockerfile != "" {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(walkPath)
			if base == ".git" || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			rel, _ := filepath.Rel(cloneDir, walkPath)
			if rel != "." && strings.Count(rel, string(os.PathSeparator)) >= 2 {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if name == "Dockerfile" || name == "dockerfile" || name == "Containerfile" {
			rel, _ := filepath.Rel(cloneDir, walkPath)
			foundDockerfile = filepath.ToSlash(rel)
			logs = append(logs, fmt.Sprintf("auto-detected Dockerfile %s", foundDockerfile))
			return filepath.SkipAll
		}
		return nil
	})
	if foundDockerfile != "" {
		return "dockerfile", foundDockerfile, logs
	}

	for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"} {
		if fileExists(filepath.Join(cloneDir, name)) {
			return "compose", name, append(logs, fmt.Sprintf("auto-detected compose %s", name))
		}
	}
	return "", "", append(logs, "no Dockerfile or compose file found in repo")
}

func isComposeName(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	return base == "compose.yaml" || base == "compose.yml" ||
		base == "docker-compose.yml" || base == "docker-compose.yaml" ||
		strings.Contains(base, "compose") && (strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml"))
}

func pickComposeService(services []string) string {
	preferred := []string{"web", "app", "frontend", "api", "server", "backend"}
	lower := map[string]string{}
	for _, svc := range services {
		lower[strings.ToLower(svc)] = svc
	}
	for _, name := range preferred {
		if svc, ok := lower[name]; ok {
			return svc
		}
	}
	return services[0]
}

func sanitizeProject(id string) string {
	id = strings.ReplaceAll(id, "-", "")
	if len(id) > 8 {
		id = id[:8]
	}
	return id
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
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
