package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	norixdb "github.com/xiron902224/norix/control-plane/internal/db"
	"github.com/xiron902224/norix/control-plane/internal/secrets"
	"github.com/xiron902224/norix/control-plane/internal/version"
)

func printCLIUsage() {
	fmt.Fprintf(os.Stderr, `Usage: norix-control-plane <command> [options]
       norix-control-plane [-migrate-only]          # run API server (default)

Commands:
  check                 Verify version, DB (if configured), and local /healthz if listening
  update [--version TAG] Download release binary, verify SHA256SUMS, replace self, restart
  uninstall [--purge] [--purge-db]  Remove bare/docker install (delegates to uninstall-norix.sh)

Env:
  NORIX_VERSION / NORIX_GITHUB_REPO / DATABASE_URL / MIGRATION_DATABASE_URL / HTTP_ADDR
  NORIX_SUPERVISE=auto|systemd|none (restart after update)
`)
}

func runCLI(args []string) int {
	if len(args) == 0 {
		return -1 // default: run API server
	}
	switch args[0] {
	case "check":
		return runCheck()
	case "update":
		return runUpdate(args[1:])
	case "uninstall":
		return runUninstall(args[1:])
	case "help":
		printCLIUsage()
		return 0
	default:
		return -1 // flags like -migrate-only → server path
	}
}

func runCheck() int {
	fmt.Printf("version=%s\n", version.Current())
	fmt.Printf("http_addr=%s\n", httpAddress())
	if err := secrets.RequireConfigured(); err != nil {
		fmt.Fprintf(os.Stderr, "secrets_key=FAIL %v\n", err)
		return 1
	}
	if secrets.InsecureDevAllowed() {
		if _, err := secrets.KeyFromEnv(); err != nil {
			fmt.Println("secrets_key=insecure-dev (plaintext allowed)")
		} else {
			fmt.Println("secrets_key=ok")
		}
	} else {
		fmt.Println("secrets_key=ok")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	migrationURL := os.Getenv("MIGRATION_DATABASE_URL")
	if migrationURL == "" {
		migrationURL = databaseURL
	}
	if databaseURL == "" && migrationURL == "" {
		fmt.Println("database=not configured (DATABASE_URL empty)")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		url := migrationURL
		if url == "" {
			url = databaseURL
		}
		pool, err := norixdb.Open(ctx, url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "database=FAIL %v\n", err)
			return 1
		}
		defer pool.Close()
		if err := pool.Ping(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "database=FAIL ping: %v\n", err)
			return 1
		}
		fmt.Println("database=ok")
	}

	addr := httpAddress()
	host := "127.0.0.1"
	port := strings.TrimPrefix(addr, ":")
	if strings.Contains(addr, ":") && !strings.HasPrefix(addr, ":") {
		parts := strings.Split(addr, ":")
		if parts[0] != "" && parts[0] != "0.0.0.0" {
			host = parts[0]
		}
		port = parts[len(parts)-1]
	}
	healthURL := fmt.Sprintf("http://%s:%s/healthz", host, port)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(healthURL)
	if err != nil {
		fmt.Printf("healthz=unreachable (%s) — ok if the server is not running yet\n", healthURL)
	} else {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		fmt.Printf("healthz=%s body=%q\n", resp.Status, strings.TrimSpace(string(body)))
		if resp.StatusCode != http.StatusOK {
			return 1
		}
	}

	fmt.Println("check=passed")
	return 0
}

func runUpdate(args []string) int {
	target := ""
	force := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--version":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--version requires a tag (e.g. v1.0.0-beta.4)")
				return 2
			}
			i++
			target = args[i]
		case "--force":
			force = true
		case "-h", "--help":
			fmt.Fprintln(os.Stderr, "usage: norix-control-plane update [--version TAG] [--force]")
			return 0
		default:
			if !strings.HasPrefix(args[i], "-") && target == "" {
				target = args[i]
			} else {
				fmt.Fprintf(os.Stderr, "unknown argument: %s\n", args[i])
				return 2
			}
		}
	}
	if target == "" {
		target = strings.TrimSpace(os.Getenv("NORIX_VERSION"))
	}
	if target == "" {
		fmt.Fprintln(os.Stderr, "set --version TAG or NORIX_VERSION")
		return 2
	}
	target = strings.TrimPrefix(strings.TrimSpace(target), "v")
	current := strings.TrimPrefix(version.Current(), "v")
	if target == current && !force {
		fmt.Printf("already at %s (pass --force to reinstall)\n", current)
		return 0
	}

	arch, err := releaseArch()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	repo := envOr("NORIX_GITHUB_REPO", "xiron902224/norix")
	tag := "v" + target
	asset := fmt.Sprintf("norix-control-plane-linux-%s", arch)
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, tag)
	binURL := base + "/" + asset
	sumsURL := base + "/SHA256SUMS"

	staging, err := os.MkdirTemp("", "norix-cp-update-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(staging)
	tmpBin := filepath.Join(staging, asset)
	tmpSums := filepath.Join(staging, "SHA256SUMS")

	fmt.Printf("downloading %s\n", binURL)
	if err := downloadFile(binURL, tmpBin); err != nil {
		fmt.Fprintf(os.Stderr, "download binary: %v\n", err)
		return 1
	}
	if err := downloadFile(sumsURL, tmpSums); err != nil {
		fmt.Fprintf(os.Stderr, "download SHA256SUMS: %v\n", err)
		return 1
	}
	if err := verifySHA256File(tmpSums, tmpBin, asset); err != nil {
		fmt.Fprintf(os.Stderr, "checksum: %v\n", err)
		return 1
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	newPath := exe + ".new"
	if err := copyFile(tmpBin, newPath, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "stage binary: %v\n", err)
		return 1
	}
	if err := os.Rename(newPath, exe); err != nil {
		fmt.Fprintf(os.Stderr, "replace binary: %v\n", err)
		return 1
	}
	patchControlPlaneEnv(target)
	fmt.Printf("updated %s → %s (%s)\n", current, target, exe)
	restartControlPlane(exe)
	return 0
}

func runUninstall(args []string) int {
	purge := false
	purgeDB := false
	mode := "auto"
	for _, a := range args {
		switch a {
		case "--purge":
			purge = true
		case "--purge-db":
			purgeDB = true
		case "--mode":
			// handled below with next — keep simple: --mode=bare
		default:
			if strings.HasPrefix(a, "--mode=") {
				mode = strings.TrimPrefix(a, "--mode=")
			}
		}
	}
	script := findUninstallNorixScript()
	if script == "" {
		fmt.Fprintln(os.Stderr, "uninstall-norix.sh not found; clone the repo or curl it from GitHub")
		fmt.Fprintln(os.Stderr, "  curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/main/scripts/uninstall-norix.sh | bash -s -- --mode bare")
		return 1
	}
	cmdArgs := []string{script, "--mode", mode}
	if purge {
		cmdArgs = append(cmdArgs, "--purge")
	}
	if purgeDB {
		cmdArgs = append(cmdArgs, "--purge-db")
	}
	cmd := exec.Command("bash", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "uninstall failed: %v\n", err)
		return 1
	}
	return 0
}

func findUninstallNorixScript() string {
	candidates := []string{
		"scripts/uninstall-norix.sh",
		"/opt/norix/scripts/uninstall-norix.sh",
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append([]string{
			filepath.Join(dir, "uninstall-norix.sh"),
			filepath.Join(dir, "..", "scripts", "uninstall-norix.sh"),
			filepath.Join(dir, "..", "..", "scripts", "uninstall-norix.sh"),
		}, candidates...)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func releaseArch() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64", nil
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported arch %s", runtime.GOARCH)
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s → %s", url, resp.Status)
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func verifySHA256File(sumsPath, binPath, assetName string) error {
	raw, err := os.ReadFile(sumsPath)
	if err != nil {
		return err
	}
	var want string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name == assetName || strings.HasSuffix(name, assetName) {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("SHA256SUMS missing %s", assetName)
	}
	f, err := os.Open(binPath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("mismatch for %s: got %s want %s", assetName, got, want)
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Chmod(mode)
}

func patchControlPlaneEnv(ver string) {
	paths := []string{
		"/etc/norix/control-plane.env",
		filepath.Join(os.Getenv("HOME"), ".norix", "control-plane.env"),
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		lines := strings.Split(string(raw), "\n")
		var out []string
		found := false
		for _, line := range lines {
			if strings.HasPrefix(line, "NORIX_VERSION=") {
				out = append(out, "NORIX_VERSION="+ver)
				found = true
			} else {
				out = append(out, line)
			}
		}
		if !found {
			out = append(out, "NORIX_VERSION="+ver)
		}
		_ = os.WriteFile(p, []byte(strings.Join(out, "\n")), 0o600)
		fmt.Printf("patched %s\n", p)
		return
	}
}

func restartControlPlane(exe string) {
	supervise := envOr("NORIX_SUPERVISE", "auto")
	if supervise == "auto" {
		if _, err := os.Stat("/run/systemd/system"); err == nil {
			supervise = "systemd"
		} else {
			supervise = "none"
		}
	}
	switch supervise {
	case "systemd":
		cmd := exec.Command("systemctl", "restart", "norix-control-plane")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "systemctl restart failed: %v — restart manually\n", err)
		} else {
			fmt.Println("restarted via systemd")
		}
	default:
		pidFile := "/var/lib/norix/norix-control-plane.pid"
		envFile := "/etc/norix/control-plane.env"
		logFile := "/var/lib/norix/norix-control-plane.log"
		script := fmt.Sprintf(`set -euo pipefail
sleep 1
if [[ -f '%s' ]]; then old=$(cat '%s' || true); [[ -n "$old" ]] && kill "$old" 2>/dev/null || true; fi
if [[ -f '%s' ]]; then set -a; source '%s'; set +a; fi
nohup '%s' >>'%s' 2>&1 &
echo $! >'%s'
`, pidFile, pidFile, envFile, envFile, exe, logFile, pidFile)
		cmd := exec.Command("bash", "-c", script)
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "spawn restart failed: %v — restart manually\n", err)
			return
		}
		fmt.Println("restart scheduled (nohup); this process may exit")
		os.Exit(0)
	}
}
