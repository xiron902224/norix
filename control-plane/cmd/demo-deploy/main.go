package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

func main() {
	ctx := context.Background()
	dsn := firstNonEmpty(os.Getenv("MIGRATION_DATABASE_URL"), os.Getenv("DATABASE_URL"))
	if dsn == "" {
		log.Fatal("DATABASE_URL or MIGRATION_DATABASE_URL required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	orgID, userID, email, err := firstOrgUser(ctx, pool)
	if err != nil {
		log.Fatal(err)
	}
	actor := db.AuditActor{UserID: userID, Email: email}

	specs := []struct {
		project, name, gitURL, ref, dockerfile, image, hostname string
		port                                                    int32
	}{
		{
			project: "blog-samples", name: "blog-react",
			gitURL: "https://github.com/dockersamples/blog-react-app.git",
			ref: "main", dockerfile: "Dockerfile.multi",
			image: "norix/blog-react:demo", port: 80, hostname: "blog.localhost",
		},
		{
			project: "hello-samples", name: "welcome-docker",
			gitURL: "https://github.com/docker/welcome-to-docker.git",
			ref: "main", dockerfile: "Dockerfile",
			image: "norix/welcome-docker:demo", port: 80, hostname: "welcome.localhost",
		},
	}

	workdir, err := os.MkdirTemp("", "norix-demo-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(workdir)

	for _, spec := range specs {
		fmt.Printf("==> building %s\n", spec.image)
		if err := buildImage(ctx, workdir, spec.gitURL, spec.ref, spec.dockerfile, spec.image); err != nil {
			log.Fatalf("build %s: %v", spec.name, err)
		}

		project, err := ensureProject(ctx, pool, orgID, spec.project, actor)
		if err != nil {
			log.Fatal(err)
		}
		workload, err := ensureImageWorkload(ctx, pool, orgID, project.ID, spec.name, spec.image, spec.gitURL, spec.ref, spec.dockerfile, spec.port, actor)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("project=%s workload=%s image=%s\n", project.Name, workload.Name, spec.image)

		_, _, err = db.DeployWorkload(ctx, pool, orgID, workload.ID, "", 0, spec.port, actor)
		if err != nil {
			log.Fatalf("deploy %s: %v", spec.name, err)
		}

		deadline := time.Now().Add(2 * time.Minute)
		for {
			wl, err := db.GetWorkload(ctx, pool, orgID, workload.ID)
			if err != nil {
				log.Fatal(err)
			}
			if wl.Status == "running" && wl.HostPort > 0 {
				fmt.Printf("  running http://127.0.0.1:%d\n", wl.HostPort)
				break
			}
			if wl.Status == "failed" {
				log.Fatalf("workload failed: %s", wl.LastError)
			}
			if time.Now().After(deadline) {
				log.Fatalf("timeout waiting for running %s (status=%s err=%s)", spec.name, wl.Status, wl.LastError)
			}
			time.Sleep(2 * time.Second)
		}

		route, err := db.AssignDomain(ctx, pool, orgID, workload.ID, spec.hostname, "internal", actor)
		if err != nil {
			log.Fatalf("assign domain %s: %v", spec.hostname, err)
		}
		fmt.Printf("  dns %s → %s\n", route.Hostname, db.DomainPublicURL(route.Hostname))
	}
}

func buildImage(ctx context.Context, workdir, gitURL, ref, dockerfile, image string) error {
	if imageExists(ctx, image) {
		fmt.Printf("  image already present, skipping build\n")
		return nil
	}
	cloneDir := filepath.Join(workdir, strings.ReplaceAll(filepath.Base(image), ":", "-"))
	_ = os.RemoveAll(cloneDir)
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", ref, gitURL, cloneDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(cloneDir)
		cmd = exec.CommandContext(ctx, "git", "clone", "--depth", "1", gitURL, cloneDir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git clone: %w", err)
		}
	}
	build := exec.CommandContext(ctx, "docker", "build", "--network=host", "-f", dockerfile, "-t", image, ".")
	build.Dir = cloneDir
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	return build.Run()
}

func imageExists(ctx context.Context, image string) bool {
	return exec.CommandContext(ctx, "docker", "image", "inspect", image).Run() == nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstOrgUser(ctx context.Context, pool *pgxpool.Pool) (orgID, userID, email string, err error) {
	err = pool.QueryRow(ctx, `
		SELECT organization_id::text, id::text, email
		FROM users ORDER BY created_at LIMIT 1`).Scan(&orgID, &userID, &email)
	return
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

func ensureImageWorkload(ctx context.Context, pool *pgxpool.Pool, orgID, projectID, name, image, gitURL, ref, dockerfile string, port int32, actor db.AuditActor) (db.Workload, error) {
	items, err := db.ListWorkloads(ctx, pool, orgID, projectID)
	if err != nil {
		return db.Workload{}, err
	}
	for _, item := range items {
		if item.Name == name {
			item.ImageOrSource = image
			item.ContainerPort = port
			item.GitURL = gitURL
			item.GitRef = ref
			item.DockerfilePath = dockerfile
			item.SourceKind = "git"
			return db.UpdateWorkload(ctx, pool, orgID, item, actor)
		}
	}
	return db.CreateWorkload(ctx, pool, orgID, db.Workload{
		ProjectID:        projectID,
		Name:             name,
		Runtime:          "container",
		SourceKind:       "git",
		GitURL:           gitURL,
		GitRef:           ref,
		DockerfilePath:   dockerfile,
		ContainerPort:    port,
		ImageOrSource:    image,
		ReplicaCount:     1,
		ScheduleStrategy: "spread",
	}, actor)
}
