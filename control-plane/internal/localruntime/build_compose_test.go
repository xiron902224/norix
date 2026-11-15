package localruntime

import "testing"

func TestExtractComposeBuild(t *testing.T) {
	yaml := `version: '3.8'
services:
  frontend:
    build:
      context: ./frontend
      dockerfile: Dockerfile
    ports:
      - "3000:3000"
  backend:
    build:
      context: ./backend
      dockerfile: Dockerfile
    ports:
      - "3001:3001"
`
	ctx, df := extractComposeBuild(yaml, "frontend")
	if ctx != "./frontend" || df != "Dockerfile" {
		t.Fatalf("frontend: got ctx=%q df=%q", ctx, df)
	}
	ctx, df = extractComposeBuild(yaml, "backend")
	if ctx != "./backend" || df != "Dockerfile" {
		t.Fatalf("backend: got ctx=%q df=%q", ctx, df)
	}
	names := listComposeServiceNames(yaml)
	if len(names) != 2 {
		t.Fatalf("services=%v", names)
	}
	picked := pickComposeService(names)
	if picked != "frontend" {
		t.Fatalf("pick=%s", picked)
	}
}

func TestExtractComposeBuildStringForm(t *testing.T) {
	yaml := `services:
  web:
    build: ./app
`
	ctx, df := extractComposeBuild(yaml, "web")
	if ctx != "./app" || df != "Dockerfile" {
		t.Fatalf("got ctx=%q df=%q", ctx, df)
	}
}
