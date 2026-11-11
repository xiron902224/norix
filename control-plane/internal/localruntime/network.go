package localruntime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/xiron902224/norix/control-plane/internal/db"
)

func (runtime *Runtime) ApplyNetwork(ctx context.Context, plan db.DesiredNetworkPlan) db.ObservedNetwork {
	logs := []string{
		fmt.Sprintf("network generation %d address=%s peers=%d dns=%d apply=%v",
			plan.Generation, plan.Address, len(plan.Peers), len(plan.DNSRecords), plan.Apply),
	}
	dir := networkStateDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return db.ObservedNetwork{
			Generation: plan.Generation, Status: "error",
			Message: err.Error(), LogLines: append(logs, err.Error()),
		}
	}

	wgPath := filepath.Join(dir, "wg-norix.conf")
	nftPath := filepath.Join(dir, "norix.nft")
	dnsPath := filepath.Join(dir, "dns-hosts")
	certDir := filepath.Join(dir, "certs")
	_ = os.MkdirAll(certDir, 0o700)

	if err := os.WriteFile(wgPath, []byte(plan.WGConfig), 0o600); err != nil {
		return failNetwork(plan, logs, err)
	}
	logs = append(logs, "wrote "+wgPath)
	if err := os.WriteFile(nftPath, []byte(plan.Nftables), 0o600); err != nil {
		return failNetwork(plan, logs, err)
	}
	logs = append(logs, "wrote "+nftPath)
	if err := os.WriteFile(dnsPath, []byte(renderHosts(plan.DNSRecords)), 0o644); err != nil {
		return failNetwork(plan, logs, err)
	}
	logs = append(logs, fmt.Sprintf("wrote %s (%d records)", dnsPath, len(plan.DNSRecords)))
	for _, cert := range plan.Certificates {
		name := sanitizeFile(cert.Hostname)
		if err := os.WriteFile(filepath.Join(certDir, name+".crt"), []byte(cert.CertPEM), 0o644); err != nil {
			return failNetwork(plan, logs, err)
		}
		if err := os.WriteFile(filepath.Join(certDir, name+".key"), []byte(cert.KeyPEM), 0o600); err != nil {
			return failNetwork(plan, logs, err)
		}
		logs = append(logs, "wrote cert "+cert.Hostname)
	}

	if !plan.Apply {
		logs = append(logs, "dry-run: set NORIX_APPLY_WIREGUARD=1 to bring up wg-norix / nft")
		return db.ObservedNetwork{
			Generation: plan.Generation,
			Status:     "ready",
			Message:    "configs written (apply disabled)",
			LogLines:   logs,
		}
	}

	if err := applyWGQuick(ctx, wgPath, plan.Interface); err != nil {
		logs = append(logs, "wg-quick: "+err.Error())
		return db.ObservedNetwork{
			Generation: plan.Generation, Status: "degraded",
			Message: err.Error(), LogLines: logs,
		}
	}
	logs = append(logs, "wg-quick applied")
	if err := applyNft(ctx, nftPath); err != nil {
		logs = append(logs, "nft: "+err.Error())
		return db.ObservedNetwork{
			Generation: plan.Generation, Status: "degraded",
			Message: err.Error(), LogLines: logs,
		}
	}
	logs = append(logs, "nftables applied")
	return db.ObservedNetwork{
		Generation: plan.Generation, Status: "ready",
		Message: "mesh applied", LogLines: logs,
	}
}

func failNetwork(plan db.DesiredNetworkPlan, logs []string, err error) db.ObservedNetwork {
	return db.ObservedNetwork{
		Generation: plan.Generation, Status: "error",
		Message: err.Error(), LogLines: append(logs, err.Error()),
	}
}

func renderHosts(records []db.NetworkDNSRecord) string {
	var b strings.Builder
	b.WriteString("# norix internal DNS snapshot\n")
	fmt.Fprintf(&b, "# generated %s\n", time.Now().UTC().Format(time.RFC3339))
	for _, record := range records {
		if record.Type != "A" && record.Type != "AAAA" {
			continue
		}
		fmt.Fprintf(&b, "%s %s\n", record.Value, record.Name)
	}
	return b.String()
}

func sanitizeFile(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "*", "wildcard")
	var b strings.Builder
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '.' {
			b.WriteRune(ch)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "cert"
	}
	return out
}

func networkStateDir() string {
	dir := os.Getenv("NORIX_NETWORK_STATE_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "norix-network")
	}
	return dir
}

func applyWGQuick(ctx context.Context, confPath, iface string) error {
	if _, err := exec.LookPath("wg-quick"); err != nil {
		return fmt.Errorf("wg-quick not installed")
	}
	if iface == "" {
		iface = "wg-norix"
	}
	_ = exec.CommandContext(ctx, "wg-quick", "down", confPath).Run()
	cmd := exec.CommandContext(ctx, "wg-quick", "up", confPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func applyNft(ctx context.Context, nftPath string) error {
	if _, err := exec.LookPath("nft"); err != nil {
		return fmt.Errorf("nft not installed")
	}
	cmd := exec.CommandContext(ctx, "nft", "-f", nftPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
