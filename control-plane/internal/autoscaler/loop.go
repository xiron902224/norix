package autoscaler

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
	"github.com/xiron902224/norix/control-plane/internal/limits"
)

// Enabled reports whether NORIX_AUTOSCALER is truthy (1/true/yes/on).
func Enabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("NORIX_AUTOSCALER")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// DefaultPolicy reads global HPA knobs from env (S18).
func DefaultPolicy() Policy {
	p := Policy{
		MinReplicas: 1,
		MaxReplicas: 8,
		CPUTarget:   70,
		RPSTarget:   0,
		CooldownMs:  30_000,
	}
	if v := envInt32("NORIX_AUTOSCALER_MIN"); v > 0 {
		p.MinReplicas = v
	}
	if v := envInt32("NORIX_AUTOSCALER_MAX"); v > 0 {
		p.MaxReplicas = v
	}
	p.MaxReplicas = limits.FromEnv().ClampAutoscalerMax(p.MaxReplicas)
	if v := envFloat("NORIX_AUTOSCALER_CPU_TARGET"); v > 0 {
		p.CPUTarget = v
	}
	if v := envFloat("NORIX_AUTOSCALER_RPS_TARGET"); v > 0 {
		p.RPSTarget = v
	}
	if v := envInt64("NORIX_AUTOSCALER_COOLDOWN_MS"); v >= 0 {
		p.CooldownMs = v
	}
	if p.MaxReplicas < p.MinReplicas {
		p.MaxReplicas = p.MinReplicas
	}
	return p
}

func envInt32(key string) int32 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}

func envInt64(key string) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return -1
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return -1
	}
	return n
}

func envFloat(key string) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return n
}

// CooldownTracker remembers last scale time per workload.
type CooldownTracker struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func NewCooldownTracker() *CooldownTracker {
	return &CooldownTracker{last: make(map[string]time.Time)}
}

// Allow returns true when cooldown has elapsed (or never scaled).
func (c *CooldownTracker) Allow(workloadID string, cooldownMs int64, now time.Time) bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	prev, ok := c.last[workloadID]
	if !ok {
		return true
	}
	if cooldownMs <= 0 {
		return true
	}
	return now.Sub(prev) >= time.Duration(cooldownMs)*time.Millisecond
}

// Mark records a successful scale.
func (c *CooldownTracker) Mark(workloadID string, now time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.last == nil {
		c.last = make(map[string]time.Time)
	}
	c.last[workloadID] = now
}

// PolicyForWorkload merges global policy with optional per-workload env overrides.
func PolicyForWorkload(base Policy, env map[string]string) Policy {
	p := base
	if env == nil {
		return p
	}
	if v := parseInt32(envValue(env, "NORIX_AUTOSCALE_MIN", "AUTOSCALE_MIN")); v > 0 {
		p.MinReplicas = v
	}
	if v := parseInt32(envValue(env, "NORIX_AUTOSCALE_MAX", "AUTOSCALE_MAX")); v > 0 {
		p.MaxReplicas = v
	}
	if v := parseFloat(envValue(env, "NORIX_AUTOSCALE_CPU_TARGET", "AUTOSCALE_CPU_TARGET")); v > 0 {
		p.CPUTarget = v
	}
	if v := parseFloat(envValue(env, "NORIX_AUTOSCALE_RPS_TARGET", "AUTOSCALE_RPS_TARGET")); v > 0 {
		p.RPSTarget = v
	}
	if raw := envValue(env, "NORIX_AUTOSCALE_COOLDOWN_MS", "AUTOSCALE_COOLDOWN_MS"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n >= 0 {
			p.CooldownMs = n
		}
	}
	if p.MaxReplicas < p.MinReplicas {
		p.MaxReplicas = p.MinReplicas
	}
	return p
}

func envValue(env map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := env[k]; ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func parseInt32(raw string) int32 {
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}

func parseFloat(raw string) float64 {
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return n
}

// WorkloadAutoscalerEnabled is true unless the workload explicitly opts out.
// When the global loop is running (NORIX_AUTOSCALER=1), workloads with metrics
// are scaled; set NORIX_AUTOSCALE=0 to exclude one. Opt-in alone (1/true) also works
// when NORIX_AUTOSCALER_ALL=0.
func WorkloadAutoscalerEnabled(env map[string]string, scaleAll bool) bool {
	v := strings.ToLower(envValue(env, "NORIX_AUTOSCALE", "AUTOSCALE"))
	if v == "0" || v == "false" || v == "no" || v == "off" {
		return false
	}
	if scaleAll {
		return true
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// ScaleAllEnabled defaults true so NORIX_AUTOSCALER=1 covers all metric-backed
// workloads; set NORIX_AUTOSCALER_ALL=0 to require per-workload opt-in.
func ScaleAllEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("NORIX_AUTOSCALER_ALL")))
	if v == "" {
		return true
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// Decide reports desired replicas and whether to apply (respects cooldown).
func Decide(p Policy, current int32, cpuPercent, rps float64, lastScale time.Time, now time.Time, everScaled bool) (desired int32, apply bool) {
	desired = DesiredReplicas(p, current, cpuPercent, rps)
	if desired == current {
		return desired, false
	}
	if everScaled && p.CooldownMs > 0 && now.Sub(lastScale) < time.Duration(p.CooldownMs)*time.Millisecond {
		return desired, false
	}
	return desired, true
}

// Start runs the HPA ticker until ctx is cancelled (leader-only).
func Start(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	tracker := NewCooldownTracker()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	runOnce(ctx, pool, logger, tracker, time.Now)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce(ctx, pool, logger, tracker, time.Now)
		}
	}
}

func runOnce(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, tracker *CooldownTracker, nowFn func() time.Time) {
	base := DefaultPolicy()
	if err := base.Validate(); err != nil {
		logger.Warn("autoscaler policy invalid", "error", err)
		return
	}
	scaleAll := ScaleAllEnabled()
	orgs, err := db.ListOrganizationIDs(ctx, pool)
	if err != nil {
		logger.Warn("autoscaler list orgs", "error", err)
		return
	}
	now := nowFn()
	actor := db.AuditActor{Email: "autoscaler"}
	for _, orgID := range orgs {
		workloads, err := db.ListScalableWorkloads(ctx, pool, orgID)
		if err != nil {
			logger.Warn("autoscaler list workloads", "org", orgID, "error", err)
			continue
		}
		for _, item := range workloads {
			if !WorkloadAutoscalerEnabled(item.Environment, scaleAll) {
				continue
			}
			policy := PolicyForWorkload(base, item.Environment)
			if err := policy.Validate(); err != nil {
				logger.Debug("autoscaler skip invalid policy", "workload", item.ID, "error", err)
				continue
			}
			cpu, rps, ok := observeSignals(ctx, pool, orgID, item)
			if !ok {
				continue
			}
			if !tracker.Allow(item.ID, policy.CooldownMs, now) {
				continue
			}
			desired := DesiredReplicas(policy, item.ReplicaCount, cpu, rps)
			if desired == item.ReplicaCount {
				continue
			}
			updated, err := db.SetWorkloadReplicaCount(ctx, pool, orgID, item.ID, desired, actor)
			if err != nil {
				logger.Warn("autoscaler set replicas", "org", orgID, "workload", item.ID, "error", err)
				continue
			}
			tracker.Mark(item.ID, now)
			if _, err := db.RescheduleOrphanReplicas(ctx, pool, orgID); err != nil {
				logger.Warn("autoscaler reschedule", "org", orgID, "workload", item.ID, "error", err)
			}
			logger.Info("autoscaler adjusted replicas",
				"org", orgID,
				"workload", item.ID,
				"name", item.Name,
				"from", item.ReplicaCount,
				"to", updated.ReplicaCount,
				"cpu", cpu,
				"rps", rps,
			)
		}
	}
}

func observeSignals(ctx context.Context, pool *pgxpool.Pool, orgID string, item db.ScalableWorkload) (cpu, rps float64, ok bool) {
	cpu = item.NodeCPUUsagePercent
	hasCPU := item.HasNodeCPU
	if sample, found, err := db.LatestMetricValue(ctx, pool, orgID, item.AssignedNodeID, "node_cpu_usage_percent", "", ""); err == nil && found {
		cpu = sample
		hasCPU = true
	}
	if sample, found, err := db.LatestMetricValue(ctx, pool, orgID, "", "workload_rps", "workload_id", item.ID); err == nil && found {
		rps = sample
	} else if sample, found, err := db.LatestMetricValue(ctx, pool, orgID, "", "workload_http_rps", "workload_id", item.ID); err == nil && found {
		rps = sample
	}
	// Need at least one usable signal matching policy targets.
	policy := PolicyForWorkload(DefaultPolicy(), item.Environment)
	if policy.CPUTarget > 0 && hasCPU {
		return cpu, rps, true
	}
	if policy.RPSTarget > 0 && rps > 0 {
		return cpu, rps, true
	}
	// When RPS target unset and we have node CPU, still OK.
	if hasCPU {
		return cpu, rps, true
	}
	return 0, 0, false
}
