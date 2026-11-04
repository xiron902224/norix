.PHONY: proto-lint proto-generate clean

proto-lint:
	buf lint

proto-generate: proto-lint
	buf generate
	git diff --exit-code proto/

clean:
	rm -rf control-plane/internal/gen/v1/*
	rm -rf web/src/gen/v1/*
