package localruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/xiron902224/norix/control-plane/internal/db"
	"github.com/xiron902224/norix/control-plane/internal/gitssrf"
)

// BuildLogFn streams live lines to the control plane while a build runs.
type BuildLogFn func(buildID, line string)

func (runtime *Runtime) RunBuilds(ctx context.Context, builds []db.DesiredBuild, onLog BuildLogFn) []db.ObservedBuild {
	observed := make([]db.ObservedBuild, 0, len(builds))
	for _, build := range builds {
		result, err := runtime.runBuild(ctx, build, onLog)
		if err != nil {
			msg := err.Error()
			emitLog(onLog, build.ID, msg)
			observed = append(observed, db.ObservedBuild{
				ID:       build.ID,
				Status:   "failed",
				Message:  msg,
				LogLines: []string{msg},
			})
			continue
		}
		observed = append(observed, result)
	}
	return observed
}

func emitLog(onLog BuildLogFn, buildID, line string) {
	line = strings.TrimSpace(line)
	if onLog == nil || line == "" {
		return
	}
	onLog(buildID, line)
}

func (runtime *Runtime) runBuild(ctx context.Context, build db.DesiredBuild, onLog BuildLogFn) (db.ObservedBuild, error) {
	logs := []string{}
	appendLog := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		logs = append(logs, line)
		emitLog(onLog, build.ID, line)
	}

	appendLog(fmt.Sprintf("building %s @ %s", build.GitURL, build.GitRef))
	if err := gitssrf.ValidateGitURL(build.GitURL); err != nil {
		appendLog(err.Error())
		return db.ObservedBuild{}, err
	}
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
	appendLog(fmt.Sprintf("cloning (depth=1, branch=%s)…", ref))
	clone := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", ref, build.GitURL, cloneDir)
	cloneOut, cloneErr := clone.CombinedOutput()
	for _, line := range truncateLines(string(cloneOut), 40) {
		appendLog(line)
	}
	if cloneErr != nil {
		_ = os.RemoveAll(cloneDir)
		appendLog("branch clone failed; retrying without --branch")
		clone = exec.CommandContext(ctx, "git", "clone", "--depth", "1", build.GitURL, cloneDir)
		cloneOut, cloneErr = clone.CombinedOutput()
		for _, line := range truncateLines(string(cloneOut), 40) {
			appendLog(line)
		}
		if cloneErr != nil {
			return db.ObservedBuild{}, fmt.Errorf("git clone: %w", cloneErr)
		}
	}
	appendLog("clone complete")

	kind, path, discoverLogs := resolveBuildSource(cloneDir, build.DockerfilePath)
	for _, line := range discoverLogs {
		appendLog(line)
	}
	if kind == "" {
		return db.ObservedBuild{ID: build.ID, Status: "failed", Message: "no Dockerfile, compose, or Railpack source found", LogLines: logs}, fmt.Errorf("no Dockerfile, compose, or Railpack source found")
	}

	var buildErr error
	switch kind {
	case "dockerfile":
		appendLog(fmt.Sprintf("using Dockerfile %s", path))
		// Prefer host npm+nginx packaging when package.json exists — avoids flaky
		// npm-inside-Docker DNS on constrained local networks.
		if hostBuilt, hostErr := runtime.tryHostNodeBuild(ctx, cloneDir, path, build, appendLog); hostBuilt {
			buildErr = hostErr
		} else {
			if hostErr != nil {
				appendLog(fmt.Sprintf("host node build skipped: %v", hostErr))
			}
			buildErr = runtime.buildDockerfile(ctx, cloneDir, path, build, appendLog)
			for attempt := 2; buildErr != nil && attempt <= 3 && isTransientBuildErr(buildErr); attempt++ {
				appendLog(fmt.Sprintf("transient build error; retry %d/3…", attempt))
				select {
				case <-ctx.Done():
					buildErr = ctx.Err()
				case <-time.After(time.Duration(attempt) * 5 * time.Second):
					buildErr = runtime.buildDockerfile(ctx, cloneDir, path, build, appendLog)
				}
			}
		}
	case "compose":
		appendLog(fmt.Sprintf("using compose file %s", path))
		rewriteNpmDockerfilesForCompose(cloneDir, appendLog)
		buildErr = runtime.buildCompose(ctx, cloneDir, path, build, appendLog)
	case "railpack":
		appendLog(fmt.Sprintf("using Railpack (%s)", path))
		buildErr = runtime.buildRailpack(ctx, cloneDir, build, appendLog)
	default:
		buildErr = fmt.Errorf("unknown build kind %s", kind)
	}
	if buildErr != nil {
		return db.ObservedBuild{ID: build.ID, Status: "failed", Message: buildErr.Error(), LogLines: logs}, buildErr
	}

	image := build.ImageTag
	if build.Registry != "" && strings.Contains(build.ImageTag, build.Registry) {
		appendLog(fmt.Sprintf("pushing %s", build.ImageTag))
		push := exec.CommandContext(ctx, string(runtime.engine), "push", build.ImageTag)
		pushOut, pushErr := push.CombinedOutput()
		for _, line := range truncateLines(string(pushOut), 40) {
			appendLog(line)
		}
		if pushErr != nil {
			appendLog(fmt.Sprintf("push failed (%v); using local image tag", pushErr))
		}
	}
	appendLog("build succeeded")
	return db.ObservedBuild{
		ID:       build.ID,
		Status:   "succeeded",
		Image:    image,
		LogLines: logs,
	}, nil
}

func (runtime *Runtime) buildDockerfile(ctx context.Context, cloneDir, dockerfile string, build db.DesiredBuild, appendLog func(string)) error {
	effectiveDockerfile, rewritten, err := maybeRewriteNpmDockerfile(cloneDir, dockerfile, appendLog)
	if err != nil {
		return err
	}
	if rewritten {
		dockerfile = effectiveDockerfile
	}
	effectiveDockerfile, cmdRewritten, err := ensureDockerfileStartCmd(cloneDir, dockerfile, appendLog)
	if err != nil {
		return err
	}
	if cmdRewritten {
		dockerfile = effectiveDockerfile
	}

	// Nested Dockerfiles (backend/Dockerfile) must build with that directory as context.
	// Using the repo root breaks COPY package*.json / COPY . . for multi-folder apps.
	buildCtx := cloneDir
	dockerfileArg := dockerfile
	if dir := filepath.Dir(dockerfile); dir != "." && dir != "" {
		buildCtx = filepath.Join(cloneDir, dir)
		dockerfileArg = filepath.Base(dockerfile)
		appendLog(fmt.Sprintf("using build context %s (dockerfile %s)", dir, dockerfileArg))
	}

	args := []string{"build", "--progress=plain", "--network=host", "-f", dockerfileArg, "-t", build.ImageTag}
	for key, value := range build.BuildArgs {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, value))
	}
	args = append(args, ".")
	appendLog(fmt.Sprintf("docker %s", strings.Join(args, " ")))
	cmd := exec.CommandContext(ctx, string(runtime.engine), args...)
	cmd.Dir = buildCtx
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1", "BUILDKIT_PROGRESS=plain")
	return streamCommand(ctx, cmd, appendLog, "docker build failed")
}

func isTransientBuildErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{"etimedout", "eai_again", "network", "npm err", "tls handshake timeout", "i/o timeout", "temporary failure", "exit status 1", "exit status 146", "exit status 71"} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// tryHostNodeBuild runs npm install/build on the host, then packages the static
// output into nginx:alpine. Returns (true, err) when this path was selected.
func (runtime *Runtime) tryHostNodeBuild(ctx context.Context, cloneDir, dockerfile string, build db.DesiredBuild, appendLog func(string)) (bool, error) {
	if _, err := os.Stat(filepath.Join(cloneDir, "package.json")); err != nil {
		return false, nil
	}
	npm, err := exec.LookPath("npm")
	if err != nil {
		return false, fmt.Errorf("npm not on PATH")
	}
	// Only for CRA/Vite static apps (production nginx Dockerfiles).
	lowerDockerfile := strings.ToLower(dockerfile)
	if !strings.Contains(lowerDockerfile, "multi") && !strings.Contains(lowerDockerfile, "nginx") {
		raw, readErr := os.ReadFile(filepath.Join(cloneDir, dockerfile))
		if readErr != nil || !strings.Contains(strings.ToLower(string(raw)), "nginx") {
			return false, nil
		}
	}
	appendLog("host path: npm install + npm run build (then nginx:alpine image)")
	install := exec.CommandContext(ctx, npm, "install", "--fetch-retries=5", "--fetch-retry-mintimeout=20000", "--fetch-retry-maxtimeout=120000")
	install.Dir = cloneDir
	if err := streamCommand(ctx, install, appendLog, "host npm install failed"); err != nil {
		return true, err
	}
	runBuild := exec.CommandContext(ctx, npm, "run", "build")
	runBuild.Dir = cloneDir
	if err := streamCommand(ctx, runBuild, appendLog, "host npm run build failed"); err != nil {
		return true, err
	}
	staticDir := ""
	// Never treat source `public/` as build output — that packages assets into
	// nginx:alpine and users see a static/nginx site instead of the Node app.
	for _, candidate := range []string{"build", "dist", "out"} {
		if fileExists(filepath.Join(cloneDir, candidate)) || dirExists(filepath.Join(cloneDir, candidate)) {
			entries, _ := os.ReadDir(filepath.Join(cloneDir, candidate))
			if len(entries) > 0 {
				staticDir = candidate
				break
			}
		}
	}
	if staticDir == "" {
		return true, fmt.Errorf("no static output dir (build/dist/out) after npm run build")
	}
	appendLog(fmt.Sprintf("packaging static dir %s into nginx:alpine", staticDir))
	df := filepath.Join(cloneDir, "Dockerfile.norix-host")
	content := fmt.Sprintf("FROM nginx:alpine\nCOPY %s /usr/share/nginx/html\nEXPOSE 80\nCMD [\"nginx\",\"-g\",\"daemon off;\"]\n", staticDir)
	if err := os.WriteFile(df, []byte(content), 0o644); err != nil {
		return true, err
	}
	args := []string{"build", "--progress=plain", "--network=host", "-f", "Dockerfile.norix-host", "-t", build.ImageTag, "."}
	appendLog(fmt.Sprintf("docker %s", strings.Join(args, " ")))
	cmd := exec.CommandContext(ctx, string(runtime.engine), args...)
	cmd.Dir = cloneDir
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1", "BUILDKIT_PROGRESS=plain")
	if err := streamCommand(ctx, cmd, appendLog, "docker package failed"); err != nil {
		return true, err
	}
	return true, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// rewriteNpmDockerfilesForCompose applies npm ci→install / retry rewrites in-place
// so `docker compose build` (which ignores *.norix sidecars) picks up the fixes.
func rewriteNpmDockerfilesForCompose(cloneDir string, appendLog func(string)) {
	_ = filepath.WalkDir(cloneDir, func(walkPath string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if name != "Dockerfile" && name != "dockerfile" && name != "Containerfile" {
			return nil
		}
		rel, relErr := filepath.Rel(cloneDir, walkPath)
		if relErr != nil {
			return nil
		}
		outName, changed, rewriteErr := maybeRewriteNpmDockerfile(cloneDir, filepath.ToSlash(rel), appendLog)
		if rewriteErr != nil || !changed {
			return nil
		}
		rewritten := filepath.Join(cloneDir, outName)
		data, readErr := os.ReadFile(rewritten)
		if readErr != nil {
			return nil
		}
		if err := os.WriteFile(walkPath, data, 0o644); err != nil {
			appendLog(fmt.Sprintf("could not apply npm rewrite to %s: %v", rel, err))
			return nil
		}
		appendLog(fmt.Sprintf("applied npm rewrite in-place for compose: %s", rel))
		return nil
	})
}

// maybeRewriteNpmDockerfile injects npm retry flags so flaky registries survive local builds.
// Also pins bare `redis` to redis@3 — many sample apps still use the v3 callback API.
// When package-lock.json is missing next to the Dockerfile, rewrites `npm ci` → `npm install`.
func maybeRewriteNpmDockerfile(cloneDir, dockerfile string, appendLog func(string)) (string, bool, error) {
	srcPath := filepath.Join(cloneDir, dockerfile)
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return dockerfile, false, err
	}
	text := string(raw)
	if !strings.Contains(text, "npm install") && !strings.Contains(text, "npm ci") {
		return dockerfile, false, nil
	}
	replaced := false
	dfDir := filepath.Join(cloneDir, filepath.Dir(dockerfile))
	hasLock := fileExists(filepath.Join(dfDir, "package-lock.json")) ||
		fileExists(filepath.Join(dfDir, "npm-shrinkwrap.json"))
	if !hasLock && strings.Contains(text, "npm ci") {
		text = strings.ReplaceAll(text, "RUN npm ci", "RUN npm install")
		replaced = true
		appendLog("no package-lock.json beside Dockerfile; rewriting npm ci → npm install")
	}
	for _, from := range []string{
		"RUN npm install",
		"RUN npm ci",
	} {
		if strings.Contains(text, from) && !strings.Contains(text, "--fetch-retries") {
			to := from + " --fetch-retries=5 --fetch-retry-mintimeout=20000 --fetch-retry-maxtimeout=120000"
			text = strings.ReplaceAll(text, from, to)
			replaced = true
		}
	}
	// Pin unversioned redis package references in RUN npm lines (not package.json copy).
	redisPin := regexp.MustCompile(`(^|\s)redis(\s|$)`)
	if redisPin.MatchString(text) && !strings.Contains(text, "redis@") {
		text = redisPin.ReplaceAllString(text, `${1}redis@3${2}`)
		replaced = true
		appendLog("pinning redis@3 for callback-API sample compatibility")
	}
	if !replaced {
		return dockerfile, false, nil
	}
	outName := dockerfile + ".norix"
	outPath := filepath.Join(cloneDir, outName)
	if err := os.WriteFile(outPath, []byte(text), 0o644); err != nil {
		return dockerfile, false, err
	}
	appendLog(fmt.Sprintf("rewrote %s → %s with npm fetch retries", dockerfile, outName))
	return outName, true, nil
}

// ensureDockerfileStartCmd appends CMD when the Dockerfile omits one.
// Samples like dockersamples/todolist-node expose 3000 but inherit `CMD ["node"]`
// from the base image, which exits immediately and never serves the app.
func ensureDockerfileStartCmd(cloneDir, dockerfile string, appendLog func(string)) (string, bool, error) {
	srcPath := filepath.Join(cloneDir, dockerfile)
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return dockerfile, false, err
	}
	text := string(raw)
	if dockerfileDefinesStart(text) {
		return dockerfile, false, nil
	}
	cmd := inferNodeStartCmd(cloneDir)
	if cmd == "" {
		return dockerfile, false, nil
	}
	appendLog(fmt.Sprintf("Dockerfile has no CMD; injecting %s", cmd))
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += cmd + "\n"
	outName := dockerfile
	if !strings.HasSuffix(outName, ".norix") && !strings.HasSuffix(outName, ".norix-cmd") {
		outName = dockerfile + ".norix-cmd"
	}
	outPath := filepath.Join(cloneDir, outName)
	if err := os.WriteFile(outPath, []byte(text), 0o644); err != nil {
		return dockerfile, false, err
	}
	return outName, true, nil
}

func dockerfileDefinesStart(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "CMD ") || strings.HasPrefix(upper, "CMD\t") ||
			strings.HasPrefix(upper, "ENTRYPOINT ") || strings.HasPrefix(upper, "ENTRYPOINT\t") {
			return true
		}
	}
	return false
}

func inferNodeStartCmd(cloneDir string) string {
	pkgPath := filepath.Join(cloneDir, "package.json")
	raw, err := os.ReadFile(pkgPath)
	if err != nil {
		return ""
	}
	var pkg struct {
		Main    string            `json:"main"`
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return ""
	}
	if start := strings.TrimSpace(pkg.Scripts["start"]); start != "" {
		// Prefer npm start so script changes in package.json keep working.
		return `CMD ["npm", "start"]`
	}
	main := strings.TrimSpace(pkg.Main)
	if main == "" {
		for _, candidate := range []string{"app.js", "server.js", "index.js"} {
			if fileExists(filepath.Join(cloneDir, candidate)) {
				main = candidate
				break
			}
		}
	}
	if main == "" {
		return ""
	}
	return fmt.Sprintf(`CMD ["node", %q]`, main)
}

func (runtime *Runtime) buildCompose(ctx context.Context, cloneDir, composeFile string, build db.DesiredBuild, appendLog func(string)) error {
	project := "norix" + sanitizeProject(build.ID)
	bin := string(runtime.engine)
	composeArgs := func(args ...string) *exec.Cmd {
		all := append([]string{"compose", "-p", project, "-f", composeFile}, args...)
		cmd := exec.CommandContext(ctx, bin, all...)
		cmd.Dir = cloneDir
		cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1", "BUILDKIT_PROGRESS=plain", "COMPOSE_DOCKER_CLI_BUILD=1")
		return cmd
	}

	servicesOut, err := composeArgs("config", "--services").CombinedOutput()
	for _, line := range truncateLines(string(servicesOut), 20) {
		appendLog(line)
	}
	if err != nil {
		legacy := exec.CommandContext(ctx, "docker-compose", "-p", project, "-f", composeFile, "config", "--services")
		legacy.Dir = cloneDir
		servicesOut, err = legacy.CombinedOutput()
		for _, line := range truncateLines(string(servicesOut), 20) {
			appendLog(line)
		}
		if err != nil {
			return fmt.Errorf("compose config failed: %w", err)
		}
		composeArgs = func(args ...string) *exec.Cmd {
			all := append([]string{"-p", project, "-f", composeFile}, args...)
			cmd := exec.CommandContext(ctx, "docker-compose", all...)
			cmd.Dir = cloneDir
			cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1", "BUILDKIT_PROGRESS=plain", "COMPOSE_DOCKER_CLI_BUILD=1")
			return cmd
		}
	}

	services := strings.Fields(string(servicesOut))
	if len(services) == 0 {
		return fmt.Errorf("compose file has no services")
	}
	service := pickComposeService(services)
	appendLog(fmt.Sprintf("building compose service %s", service))

	// Prefer a direct `docker build --network=host` of the service Dockerfile.
	// `docker compose build` often fails npm DNS on constrained networks and
	// names images inconsistently (project-service vs project_service vs bake).
	if dfRel, ok := composeServiceDockerfile(cloneDir, composeFile, service); ok {
		appendLog(fmt.Sprintf("compose→dockerfile path %s (network=host)", dfRel))
		return runtime.buildDockerfile(ctx, cloneDir, dfRel, build, appendLog)
	}

	// Inject build.network=host into a temp compose override when possible.
	overridePath := filepath.Join(cloneDir, "docker-compose.norix-build.yml")
	if err := writeComposeHostNetworkOverride(overridePath, services); err == nil {
		appendLog("using compose override with build.network=host")
		orig := composeArgs
		composeArgs = func(args ...string) *exec.Cmd {
			withOverride := append([]string{"-f", filepath.Base(overridePath)}, args...)
			return orig(withOverride...)
		}
	}

	buildCmd := composeArgs("build", "--progress=plain", service)
	buildErr := streamCommand(ctx, buildCmd, appendLog, "compose build failed")
	if buildErr != nil {
		appendLog(fmt.Sprintf("compose build failed: %v; trying pull for image-based services", buildErr))
		upCmd := composeArgs("pull", service)
		_ = streamCommand(ctx, upCmd, appendLog, "compose pull failed")
	}

	candidates := composeImageCandidates(project, service, composeArgs)
	var lastTagErr error
	for _, localImage := range candidates {
		if localImage == "" {
			continue
		}
		appendLog(fmt.Sprintf("trying tag %s → %s", localImage, build.ImageTag))
		tagCmd := exec.CommandContext(ctx, string(runtime.engine), "tag", localImage, build.ImageTag)
		if out, tagErr := tagCmd.CombinedOutput(); tagErr != nil {
			lastTagErr = tagErr
			for _, line := range truncateLines(string(out), 5) {
				appendLog(line)
			}
			continue
		}
		appendLog(fmt.Sprintf("tagged image %s", build.ImageTag))
		return nil
	}
	if buildErr != nil {
		return fmt.Errorf("compose build failed: %w", buildErr)
	}
	if lastTagErr != nil {
		return fmt.Errorf("compose image tag failed for %s: %w", strings.Join(candidates, " / "), lastTagErr)
	}
	return fmt.Errorf("compose produced no image for service %s", service)
}

// composeServiceDockerfile resolves context/dockerfile for a named compose service.
func composeServiceDockerfile(cloneDir, composeFile, service string) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(cloneDir, composeFile))
	if err != nil {
		return "", false
	}
	ctx, df := extractComposeBuild(string(raw), service)
	if ctx == "" && df == "" {
		return "", false
	}
	if df == "" {
		df = "Dockerfile"
	}
	ctx = strings.TrimPrefix(strings.TrimSpace(ctx), "./")
	if ctx == "" || ctx == "." {
		if fileExists(filepath.Join(cloneDir, df)) {
			return df, true
		}
		return "", false
	}
	rel := filepath.ToSlash(filepath.Join(ctx, df))
	if fileExists(filepath.Join(cloneDir, rel)) {
		return rel, true
	}
	return "", false
}

func extractComposeBuild(composeYAML, service string) (contextDir, dockerfile string) {
	// Find the service block and its build: key without a full YAML dependency
	// in this package path — keep it resilient to common compose shapes.
	lines := strings.Split(composeYAML, "\n")
	inServices := false
	inTarget := false
	inBuild := false
	serviceIndent := -1
	buildIndent := -1
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if !inServices {
			if trimmed == "services:" {
				inServices = true
			}
			continue
		}
		if indent == 0 && strings.HasSuffix(trimmed, ":") && trimmed != "services:" {
			break
		}
		if !inTarget {
			if indent == 2 && strings.TrimSuffix(trimmed, ":") == service {
				inTarget = true
				serviceIndent = indent
			}
			continue
		}
		if indent <= serviceIndent && strings.HasSuffix(trimmed, ":") {
			break
		}
		if !inBuild {
			if strings.HasPrefix(trimmed, "build:") {
				rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "build:"))
				if rest != "" && !strings.HasPrefix(rest, "|") && !strings.HasPrefix(rest, ">") {
					return strings.Trim(rest, `"'`), "Dockerfile"
				}
				inBuild = true
				buildIndent = indent
			}
			continue
		}
		if indent <= buildIndent {
			break
		}
		if strings.HasPrefix(trimmed, "context:") {
			contextDir = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "context:")), `"'`)
		}
		if strings.HasPrefix(trimmed, "dockerfile:") {
			dockerfile = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "dockerfile:")), `"'`)
		}
	}
	return contextDir, dockerfile
}

func writeComposeHostNetworkOverride(path string, services []string) error {
	var b strings.Builder
	b.WriteString("services:\n")
	for _, svc := range services {
		b.WriteString("  ")
		b.WriteString(svc)
		b.WriteString(":\n    build:\n      network: host\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func composeImageCandidates(project, service string, composeArgs func(args ...string) *exec.Cmd) []string {
	candidates := []string{
		fmt.Sprintf("%s-%s", project, service),
		fmt.Sprintf("%s_%s", project, service),
		fmt.Sprintf("%s-%s:latest", project, service),
		fmt.Sprintf("%s_%s:latest", project, service),
	}
	if out, err := composeArgs("images", "-q", service).CombinedOutput(); err == nil {
		for _, id := range strings.Fields(string(out)) {
			candidates = append([]string{id}, candidates...)
		}
	}
	if out, err := composeArgs("config", "--images", service).CombinedOutput(); err == nil {
		for _, img := range strings.Fields(string(out)) {
			candidates = append(candidates, img)
		}
	}
	// Dedup while preserving order.
	seen := map[string]struct{}{}
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

func streamCommand(ctx context.Context, cmd *exec.Cmd, appendLog func(string), failPrefix string) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	scan := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			appendLog(scanner.Text())
		}
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)
	wg.Wait()
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%s: %w", failPrefix, ctx.Err())
		}
		return fmt.Errorf("%s: %w", failPrefix, err)
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
			// Prefer production multi-stage image when user left default "Dockerfile"
			// but Dockerfile.multi exists (e.g. dockersamples/blog-react-app).
			if strings.EqualFold(filepath.Base(hint), "Dockerfile") {
				multi := filepath.Join(filepath.Dir(candidate), "Dockerfile.multi")
				if fileExists(multi) {
					rel := filepath.ToSlash(filepath.Join(filepath.Dir(hint), "Dockerfile.multi"))
					if filepath.Dir(hint) == "." || filepath.Dir(hint) == "" {
						rel = "Dockerfile.multi"
					}
					return "dockerfile", rel, []string{
						fmt.Sprintf("hint %s present; preferring Dockerfile.multi (smaller runtime image)", hint),
					}
				}
			}
			return "dockerfile", hint, []string{fmt.Sprintf("using hinted Dockerfile %s", hint)}
		}
		logs = append(logs, fmt.Sprintf("hinted path %s not found; auto-detecting", hint))
	}

	// Prefer multi-stage / alpine production Dockerfiles when present at repo root.
	for _, name := range []string{"Dockerfile.multi", "Dockerfile.prod", "Dockerfile", "dockerfile", "Containerfile", "docker/Dockerfile", "build/Dockerfile", ".docker/Dockerfile"} {
		if fileExists(filepath.Join(cloneDir, name)) {
			return "dockerfile", name, append(logs, fmt.Sprintf("auto-detected Dockerfile %s", name))
		}
	}

	// Multi-service samples (frontend+backend) usually ship compose at root and
	// nested Dockerfiles. Prefer the primary service's nested Dockerfile so the
	// single-workload path uses `docker build --network=host` (reliable npm DNS
	// + direct -t tagging). Explicit compose hints still use buildCompose.
	for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"} {
		if !fileExists(filepath.Join(cloneDir, name)) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(cloneDir, name))
		if err == nil {
			services := listComposeServiceNames(string(raw))
			if len(services) > 0 {
				svc := pickComposeService(services)
				if dfRel, ok := composeServiceDockerfile(cloneDir, name, svc); ok {
					return "dockerfile", dfRel, append(logs,
						fmt.Sprintf("auto-detected compose %s → building service %s via %s", name, svc, dfRel))
				}
			}
		}
		return "compose", name, append(logs, fmt.Sprintf("auto-detected compose %s (no root Dockerfile)", name))
	}

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
		if name == "Dockerfile" || name == "dockerfile" || name == "Containerfile" || name == "Dockerfile.multi" {
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

	if kind, reason, ok := detectRailpackSource(cloneDir); ok {
		return kind, reason, append(logs, "no Dockerfile/compose; falling back to Railpack for "+reason)
	}

	return "", "", append(logs, "no Dockerfile, compose file, or Railpack-detectable source found in repo")
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

func listComposeServiceNames(composeYAML string) []string {
	lines := strings.Split(composeYAML, "\n")
	inServices := false
	var names []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if !inServices {
			if trimmed == "services:" {
				inServices = true
			}
			continue
		}
		if indent == 0 && strings.HasSuffix(trimmed, ":") {
			break
		}
		if indent == 2 && strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, " ") {
			names = append(names, strings.TrimSuffix(trimmed, ":"))
		}
	}
	return names
}

func sanitizeProject(id string) string {
	id = strings.ReplaceAll(id, "-", "")
	if len(id) > 12 {
		id = id[:12]
	}
	return id
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func truncateLines(raw string, max int) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= max {
			break
		}
	}
	return out
}
