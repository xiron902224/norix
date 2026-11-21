package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
)

// desiredSnapshot is the apply payload shared by Heartbeat and WatchDesired (#44).
type desiredSnapshot struct {
	Workloads         []*paasv1.DesiredWorkload
	Builds            []*paasv1.DesiredBuild
	Edge              *paasv1.DesiredEdgePlan
	Network           *paasv1.DesiredNetworkPlan
	Wasm              *paasv1.DesiredWasmPlan
	AgentVersion      string
	ManagedDatabases  []*paasv1.DesiredManagedDatabase
	ComposeStacks     []*paasv1.DesiredComposeStack
	Fingerprint       string
}

func loadDesiredSnapshot(ctx context.Context, pool *pgxpool.Pool, organizationID, nodeID string, desiredAgentVersion string) (desiredSnapshot, error) {
	var out desiredSnapshot
	desired, err := db.ListDesiredWorkloadsForNode(ctx, pool, organizationID, nodeID)
	if err != nil {
		return out, fmt.Errorf("list desired workloads: %w", err)
	}
	out.Workloads = make([]*paasv1.DesiredWorkload, 0, len(desired))
	for _, item := range desired {
		out.Workloads = append(out.Workloads, desiredWorkloadToProto(item))
	}
	builds, err := db.ListDesiredBuildsForNode(ctx, pool, organizationID, nodeID)
	if err != nil {
		return out, fmt.Errorf("list desired builds: %w", err)
	}
	out.Builds = make([]*paasv1.DesiredBuild, 0, len(builds))
	for _, item := range builds {
		out.Builds = append(out.Builds, &paasv1.DesiredBuild{
			Id: item.ID, WorkloadId: item.WorkloadID, GitUrl: item.GitURL, GitRef: item.GitRef,
			DockerfilePath: item.DockerfilePath, ImageTag: item.ImageTag, BuildArgs: item.BuildArgs,
			Registry: item.Registry,
		})
	}
	edgePlan, err := db.GetDesiredEdgePlan(ctx, pool, organizationID, nodeID)
	if err != nil {
		return out, fmt.Errorf("desired edge plan: %w", err)
	}
	if edgePlan.Generation > 0 && len(edgePlan.Routes) > 0 {
		routes := make([]*paasv1.DesiredEdgeRoute, 0, len(edgePlan.Routes))
		for _, route := range edgePlan.Routes {
			routes = append(routes, &paasv1.DesiredEdgeRoute{
				DomainId: route.DomainID, Hostname: route.Hostname, Upstream: route.Upstream,
				TlsMode: route.TLSMode, EdgeGeneration: route.EdgeGeneration, Action: route.Action,
			})
		}
		out.Edge = &paasv1.DesiredEdgePlan{
			Generation: edgePlan.Generation, Routes: routes,
			AcmeCa: edgePlan.ACMECA, CaddyAdminUrl: edgePlan.CaddyAdminURL,
		}
	}
	networkPlan, err := db.GetDesiredNetworkPlan(ctx, pool, organizationID, nodeID)
	if err != nil {
		return out, fmt.Errorf("desired network plan: %w", err)
	}
	if networkPlan.Generation > 0 {
		out.Network = desiredNetworkToProto(networkPlan)
	}
	wasmFns, err := db.ListWasmFunctions(ctx, pool, organizationID, "")
	if err != nil {
		return out, fmt.Errorf("list wasm functions: %w", err)
	}
	if len(wasmFns) > 0 {
		fns := make([]*paasv1.DesiredWasmFunction, 0, len(wasmFns))
		for _, fn := range wasmFns {
			fns = append(fns, &paasv1.DesiredWasmFunction{
				Id: fn.ID, ProjectId: fn.ProjectID, Name: fn.Name, ImageRef: fn.ImageRef,
				FuelLimit: fn.FuelLimit, MemoryLimitMb: fn.MemoryLimitMB,
			})
		}
		// Stable generation for fingerprinting (Heartbeat historically used wall-clock).
		out.Wasm = &paasv1.DesiredWasmPlan{
			Generation: wasmContentGeneration(fns),
			Functions:  fns,
		}
	}
	out.AgentVersion = strings.TrimSpace(desiredAgentVersion)
	desiredDBs, err := db.ListDesiredManagedDatabasesForNode(ctx, pool, organizationID, nodeID)
	if err != nil {
		return out, fmt.Errorf("list desired managed databases: %w", err)
	}
	out.ManagedDatabases = make([]*paasv1.DesiredManagedDatabase, 0, len(desiredDBs))
	for _, item := range desiredDBs {
		out.ManagedDatabases = append(out.ManagedDatabases, &paasv1.DesiredManagedDatabase{
			Id: item.ID, ProjectId: item.ProjectID, Name: item.Name, Engine: item.Engine,
			Image: item.Image, EnvVars: item.EnvVars, Command: item.Command,
			HostPort: item.HostPort, ContainerPort: item.ContainerPort,
			VolumeName: item.VolumeName, VolumeMountPath: item.VolumeMountPath, Action: item.Action,
		})
	}
	if _, err := db.ClaimComposeStacksForNode(ctx, pool, organizationID, nodeID); err != nil {
		return out, fmt.Errorf("claim compose stacks: %w", err)
	}
	stacks, err := db.ListDesiredComposeStacksForNode(ctx, pool, organizationID, nodeID)
	if err != nil {
		return out, fmt.Errorf("list desired compose stacks: %w", err)
	}
	out.ComposeStacks = make([]*paasv1.DesiredComposeStack, 0, len(stacks))
	for _, stack := range stacks {
		out.ComposeStacks = append(out.ComposeStacks, &paasv1.DesiredComposeStack{
			Id: stack.ID, ProjectId: stack.ProjectID, Name: stack.Name, GitUrl: stack.GitURL, GitRef: stack.GitRef,
			ComposePath: stack.ComposePath, ComposeProject: stack.ComposeProject(),
			Action: stack.DesiredAction, Generation: stack.DesiredGeneration,
			PublicServices: stack.PublicServices, Env: stack.Env, Domains: stack.Domains,
			PurgeVolumes: stack.PurgeVolumes,
		})
	}
	out.Fingerprint = fingerprintDesired(out)
	return out, nil
}

func wasmContentGeneration(fns []*paasv1.DesiredWasmFunction) int64 {
	h := sha256.New()
	for _, fn := range fns {
		fmt.Fprintf(h, "%s|%s|%s|%d|%d\n", fn.GetId(), fn.GetImageRef(), fn.GetName(), fn.GetFuelLimit(), fn.GetMemoryLimitMb())
	}
	sum := h.Sum(nil)
	var n int64
	for i := 0; i < 8; i++ {
		n = (n << 8) | int64(sum[i])
	}
	if n < 0 {
		n = -n
	}
	if n == 0 {
		n = 1
	}
	return n
}

func fingerprintDesired(snap desiredSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "agent:%s\n", snap.AgentVersion)
	type wlKey struct {
		id  string
		gen int64
		act string
		img string
	}
	wls := make([]wlKey, 0, len(snap.Workloads))
	for _, w := range snap.Workloads {
		wls = append(wls, wlKey{w.GetId(), w.GetDesiredGeneration(), w.GetAction(), w.GetImage()})
	}
	sort.Slice(wls, func(i, j int) bool { return wls[i].id < wls[j].id })
	for _, w := range wls {
		fmt.Fprintf(h, "wl:%s:%d:%s:%s\n", w.id, w.gen, w.act, w.img)
	}
	bids := make([]string, 0, len(snap.Builds))
	for _, b := range snap.Builds {
		bids = append(bids, fmt.Sprintf("%s:%s:%s", b.GetId(), b.GetImageTag(), b.GetGitRef()))
	}
	sort.Strings(bids)
	for _, b := range bids {
		fmt.Fprintf(h, "build:%s\n", b)
	}
	if snap.Edge != nil {
		fmt.Fprintf(h, "edge:%d:%d\n", snap.Edge.GetGeneration(), len(snap.Edge.GetRoutes()))
	}
	if snap.Network != nil {
		fmt.Fprintf(h, "net:%d\n", snap.Network.GetGeneration())
	}
	if snap.Wasm != nil {
		fmt.Fprintf(h, "wasm:%d:%d\n", snap.Wasm.GetGeneration(), len(snap.Wasm.GetFunctions()))
	}
	mdb := make([]string, 0, len(snap.ManagedDatabases))
	for _, d := range snap.ManagedDatabases {
		mdb = append(mdb, fmt.Sprintf("%s:%s:%s", d.GetId(), d.GetAction(), d.GetImage()))
	}
	sort.Strings(mdb)
	for _, d := range mdb {
		fmt.Fprintf(h, "mdb:%s\n", d)
	}
	stacks := make([]string, 0, len(snap.ComposeStacks))
	for _, s := range snap.ComposeStacks {
		stacks = append(stacks, fmt.Sprintf("%s:%d:%s", s.GetId(), s.GetGeneration(), s.GetAction()))
	}
	sort.Strings(stacks)
	for _, s := range stacks {
		fmt.Fprintf(h, "compose:%s\n", s)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// waitForDesiredChange blocks until fingerprint != known, hub notifies, ctx cancels, or timeout.
// pollEvery is a safety net for multi-replica CPs that miss in-process Notify.
func waitForDesiredChange(
	ctx context.Context,
	pool *pgxpool.Pool,
	organizationID, nodeID, knownFingerprint string,
	timeout time.Duration,
	notify <-chan struct{},
	pollEvery time.Duration,
) (desiredSnapshot, bool /*changed*/, bool /*timedOut*/, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()

	load := func() (desiredSnapshot, error) {
		version := ""
		if node, err := db.GetNode(ctx, pool, organizationID, nodeID); err == nil {
			version = node.DesiredAgentVersion
		}
		return loadDesiredSnapshot(ctx, pool, organizationID, nodeID, version)
	}

	snap, err := load()
	if err != nil {
		return snap, false, false, err
	}
	if knownFingerprint == "" || snap.Fingerprint != knownFingerprint {
		return snap, true, false, nil
	}

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return snap, false, true, nil
		}
		timer := time.NewTimer(remaining)
		select {
		case <-ctx.Done():
			timer.Stop()
			return snap, false, false, ctx.Err()
		case <-notify:
			timer.Stop()
		case <-ticker.C:
			timer.Stop()
		case <-timer.C:
			snap, err = load()
			if err != nil {
				return snap, false, false, err
			}
			if snap.Fingerprint != knownFingerprint {
				return snap, true, false, nil
			}
			return snap, false, true, nil
		}
		snap, err = load()
		if err != nil {
			return snap, false, false, err
		}
		if snap.Fingerprint != knownFingerprint {
			return snap, true, false, nil
		}
	}
}
