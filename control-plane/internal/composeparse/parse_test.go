package composeparse

import "testing"

func TestParseYAMLTodoApp(t *testing.T) {
	raw := []byte(`
services:
  frontend:
    build:
      context: ./frontend
      dockerfile: Dockerfile
    ports:
      - "3000:3000"
    depends_on:
      - backend
  backend:
    build: ./backend
    ports:
      - "3001:3001"
    environment:
      - NODE_ENV=production
`)
	stack, err := ParseYAML("docker-compose.yml", raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(stack.Services) != 2 {
		t.Fatalf("services=%d", len(stack.Services))
	}
	byName := map[string]Service{}
	for _, svc := range stack.Services {
		byName[svc.Name] = svc
	}
	fe := byName["frontend"]
	if fe.BuildContext != "./frontend" || fe.Dockerfile != "Dockerfile" {
		t.Fatalf("frontend build %+v", fe)
	}
	if fe.ContainerPort != 3000 || fe.HostPort != 3000 {
		t.Fatalf("frontend ports %+v", fe)
	}
	if len(fe.DependsOn) != 1 || fe.DependsOn[0] != "backend" {
		t.Fatalf("depends %+v", fe.DependsOn)
	}
	be := byName["backend"]
	if !be.HasBuild || be.BuildContext != "./backend" {
		t.Fatalf("backend build %+v", be)
	}
	if be.Environment["NODE_ENV"] != "production" {
		t.Fatalf("env %+v", be.Environment)
	}
}
