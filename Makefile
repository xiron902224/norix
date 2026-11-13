.PHONY: proto proto-lint proto-generate build control-plane web embed-web agent test clean

proto: proto-lint proto-generate

proto-lint:
	buf lint

proto-generate: proto-lint
	buf generate
	git diff --exit-code proto/

web:
	cd web && npm ci && npm run build

embed-web: web
	rm -rf control-plane/internal/webui/dist
	mkdir -p control-plane/internal/webui/dist
	cp -R web/dist/. control-plane/internal/webui/dist/

build: embed-web control-plane agent

control-plane:
	@echo "Compilando Control Plane (Go)..."
	cd control-plane && go mod download && CGO_ENABLED=0 go build -o bin/norix-control-plane ./cmd/server/main.go

agent:
	@echo "Compilando agente Rust..."
	cd agent && cargo build --release

test:
	cd control-plane && go test ./...
	cd web && npm run build
	cd agent && cargo test

clean:
	rm -rf control-plane/bin/norix-control-plane
	rm -rf web/dist
	rm -rf control-plane/internal/webui/dist
	rm -rf agent/target
	mkdir -p control-plane/internal/webui/dist
	printf '%s\n' '<!doctype html><html><body><p>Norix placeholder UI</p></body></html>' > control-plane/internal/webui/dist/index.html
