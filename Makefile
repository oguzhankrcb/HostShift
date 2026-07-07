GO_CACHE ?= $(PWD)/.cache/go-build
GO_MOD_CACHE ?= $(PWD)/.cache/go-mod
GO_ENV := GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE)

.PHONY: build test test-go test-node test-integration-docker docker-pull-fixtures test-e2e-vm checksum sbom release-snapshot

build:
	mkdir -p dist
	mkdir -p $(GO_CACHE)
	mkdir -p $(GO_MOD_CACHE)
	$(GO_ENV) go build -o dist/hostshift ./cmd/hostshift

test: test-go

test-node:
	@echo "No root Node test suite remains; use make test-go."

test-go:
	mkdir -p $(GO_CACHE)
	mkdir -p $(GO_MOD_CACHE)
	$(GO_ENV) go test ./...

test-integration-docker:
	bash tests/integration/docker/run-matrix.sh

docker-pull-fixtures:
	bash tests/integration/docker/run-matrix.sh --pull-images

test-e2e-vm:
	bash tests/e2e/vm/run-vm-e2e.sh

checksum: build
	cd dist && shasum -a 256 hostshift > checksums.txt

sbom: build
	mkdir -p dist
	$(GO_ENV) ./dist/hostshift sbom --output dist/hostshift.sbom.spdx.json

release-snapshot:
	if command -v goreleaser >/dev/null 2>&1; then \
		goreleaser release --snapshot --clean; \
	else \
		$(MAKE) build checksum sbom; \
	fi
