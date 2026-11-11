package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

func main() {
	ctx := context.Background()
	dsn := strings.TrimSpace(os.Getenv("MIGRATION_DATABASE_URL"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dsn == "" {
		log.Fatal("DATABASE_URL required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	var orgID, userID, email string
	if err := pool.QueryRow(ctx, `
		SELECT organization_id::text, id::text, email FROM users ORDER BY created_at LIMIT 1`).Scan(&orgID, &userID, &email); err != nil {
		log.Fatal(err)
	}
	actor := db.AuditActor{UserID: userID, Email: email}

	project, err := ensureProject(ctx, pool, orgID, "smoke-lab", actor)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("==> deploy nginx image")
	wl, err := db.CreateWorkload(ctx, pool, orgID, db.Workload{
		ProjectID: project.ID, Name: fmt.Sprintf("smoke-nginx-%d", time.Now().Unix()%100000),
		Runtime: "container", SourceKind: "image", ImageOrSource: "nginx:alpine",
		ContainerPort: 80, ReplicaCount: 1, ScheduleStrategy: "spread",
	}, actor)
	if err != nil {
		log.Fatal(err)
	}
	if _, _, err := db.DeployWorkload(ctx, pool, orgID, wl.ID, "", 0, 80, actor); err != nil {
		log.Fatalf("deploy: %v", err)
	}
	hostPort := waitRunning(ctx, pool, orgID, wl.ID, 90*time.Second)
	fmt.Printf("  running on :%d\n", hostPort)
	if err := curlOK(fmt.Sprintf("http://127.0.0.1:%d/", hostPort)); err != nil {
		log.Fatalf("http check: %v", err)
	}
	fmt.Println("  http ok")

	replicaID := lookupReplicaID(ctx, pool, orgID, wl.ID)
	name := containerName(replicaID)
	fmt.Printf("==> delete workload (container %s)\n", name)
	if err := db.DeleteWorkload(ctx, pool, orgID, wl.ID, actor); err != nil {
		log.Fatalf("delete: %v", err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		items, err := db.ListWorkloads(ctx, pool, orgID, project.ID)
		if err != nil {
			log.Fatal(err)
		}
		found := false
		for _, item := range items {
			if item.ID == wl.ID {
				found = true
			}
		}
		_, _ = db.PurgeDeletingWorkloads(ctx, pool, orgID)
		gone := exec.Command("docker", "inspect", name).Run() != nil
		if !found && gone {
			fmt.Println("  removed from list and docker")
			break
		}
		if time.Now().After(deadline) {
			log.Fatalf("delete lag: inList=%v containerGone=%v name=%s", found, gone, name)
		}
		time.Sleep(2 * time.Second)
	}

	fmt.Println("==> git-style image deploy (prebuilt local tag)")
	image := "norix/smoke-static:demo"
	if err := ensureStaticImage(ctx, image); err != nil {
		log.Fatal(err)
	}
	gitWL, err := db.CreateWorkload(ctx, pool, orgID, db.Workload{
		ProjectID: project.ID, Name: fmt.Sprintf("smoke-static-%d", time.Now().Unix()%100000),
		Runtime: "container", SourceKind: "git",
		GitURL: "https://example.com/local-static", GitRef: "main", DockerfilePath: "Dockerfile",
		ImageOrSource: image, ContainerPort: 80, ReplicaCount: 1, ScheduleStrategy: "spread",
	}, actor)
	if err != nil {
		log.Fatal(err)
	}
	if _, _, err := db.DeployWorkload(ctx, pool, orgID, gitWL.ID, "", 0, 80, actor); err != nil {
		log.Fatalf("deploy static: %v", err)
	}
	port := waitRunning(ctx, pool, orgID, gitWL.ID, 90*time.Second)
	fmt.Printf("  static running on :%d\n", port)
	route, err := db.AssignDomain(ctx, pool, orgID, gitWL.ID, "smoke.localhost", "internal", actor)
	if err != nil {
		log.Fatalf("dns: %v", err)
	}
	fmt.Printf("  dns %s → %s\n", route.Hostname, db.DomainPublicURL(route.Hostname))
	fmt.Println("SMOKE OK")
}

func lookupReplicaID(ctx context.Context, pool *pgxpool.Pool, orgID, workloadID string) string {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return workloadID
	}
	defer tx.Rollback(ctx)
	if err := db.WithTenant(ctx, tx, orgID); err != nil {
		return workloadID
	}
	var id string
	if err := tx.QueryRow(ctx, `
		SELECT id::text FROM workload_replicas
		WHERE workload_id = $1::uuid AND replica_index = 0`, workloadID).Scan(&id); err != nil {
		return workloadID
	}
	return id
}

func ensureProject(ctx context.Context, pool *pgxpool.Pool, orgID, name string, actor db.AuditActor) (db.Project, error) {
	projects, err := db.ListProjects(ctx, pool, orgID)
	if err != nil {
		return db.Project{}, err
	}
	for _, project := range projects {
		if project.Name == name {
			return project, nil
		}
	}
	return db.CreateProject(ctx, pool, orgID, name, actor)
}

func waitRunning(ctx context.Context, pool *pgxpool.Pool, orgID, workloadID string, timeout time.Duration) int32 {
	deadline := time.Now().Add(timeout)
	for {
		wl, err := db.GetWorkload(ctx, pool, orgID, workloadID)
		if err != nil {
			log.Fatal(err)
		}
		if wl.Status == "running" && wl.HostPort > 0 {
			return wl.HostPort
		}
		if wl.Status == "failed" {
			log.Fatalf("failed: %s", wl.LastError)
		}
		if time.Now().After(deadline) {
			log.Fatalf("timeout status=%s err=%s", wl.Status, wl.LastError)
		}
		time.Sleep(2 * time.Second)
	}
}

func curlOK(url string) error {
	cmd := exec.Command("curl", "-fsS", "-o", "/dev/null", "-w", "%{http_code}", url)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v (%s)", err, strings.TrimSpace(string(out)))
	}
	if strings.TrimSpace(string(out)) != "200" {
		return fmt.Errorf("status %s", out)
	}
	return nil
}

func containerName(id string) string {
	clean := strings.ReplaceAll(id, "-", "")
	if len(clean) > 12 {
		clean = clean[:12]
	}
	return "norix-" + clean
}

func ensureStaticImage(ctx context.Context, image string) error {
	if exec.CommandContext(ctx, "docker", "image", "inspect", image).Run() == nil {
		return nil
	}
	dir, err := os.MkdirTemp("", "norix-static-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(dir+"/index.html", []byte("<h1>norix smoke</h1>\n"), 0o644); err != nil {
		return err
	}
	dockerfile := "FROM nginx:alpine\nCOPY index.html /usr/share/nginx/html/index.html\n"
	if err := os.WriteFile(dir+"/Dockerfile", []byte(dockerfile), 0o644); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "docker", "build", "--network=host", "-t", image, ".")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
