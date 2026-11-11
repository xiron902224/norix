package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/auth"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

func TestRLSCrossTenantIsolation(t *testing.T) {
	migrationURL := os.Getenv("NORIX_TEST_DATABASE_URL")
	appURL := os.Getenv("NORIX_TEST_APP_DATABASE_URL")
	if migrationURL == "" || appURL == "" {
		t.Skip("NORIX_TEST_DATABASE_URL and NORIX_TEST_APP_DATABASE_URL required")
	}

	ctx := context.Background()
	if err := db.Migrate(migrationURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	adminPool, err := db.Open(ctx, migrationURL)
	if err != nil {
		t.Fatalf("open admin pool: %v", err)
	}
	defer adminPool.Close()

	appPool, err := db.Open(ctx, appURL)
	if err != nil {
		t.Fatalf("open app pool: %v", err)
	}
	defer appPool.Close()

	orgA, userA := seedTenant(t, ctx, adminPool, "Org A", "owner-a@example.com")
	orgB, _ := seedTenant(t, ctx, adminPool, "Org B", "owner-b@example.com")

	project, err := db.CreateProject(ctx, appPool, orgA, "alpha", db.AuditActor{UserID: userA, Email: "owner-a@example.com"})
	if err != nil {
		t.Fatalf("create project A: %v", err)
	}

	projectsB, err := db.ListProjects(ctx, appPool, orgB)
	if err != nil {
		t.Fatalf("list projects B: %v", err)
	}
	for _, item := range projectsB {
		if item.ID == project.ID {
			t.Fatalf("tenant B listed tenant A project %s", project.ID)
		}
	}

	projectsA, err := db.ListProjects(ctx, appPool, orgA)
	if err != nil {
		t.Fatalf("list projects A: %v", err)
	}
	found := false
	for _, item := range projectsA {
		if item.ID == project.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("tenant A did not see its own project")
	}
}

func seedTenant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgName, email string) (organizationID, userID string) {
	t.Helper()
	passwordHash, err := auth.HashPassword("integration-test-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	slug := orgName + "-" + time.Now().UTC().Format("150405.000")
	if err := tx.QueryRow(ctx, `
		INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id::text`, orgName, slug).Scan(&organizationID); err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (organization_id, email, email_normalized, name, role, password_hash)
		VALUES ($1::uuid, $2, $2, $3, 'owner', $4)
		RETURNING id::text`, organizationID, email, orgName+" Owner", passwordHash).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return organizationID, userID
}
