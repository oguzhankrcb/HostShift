.PHONY: build test test-go test-node test-integration-docker docker-pull-fixtures test-e2e-vm checksum sbom release-snapshot

build:
	mkdir -p dist
	mkdir -p .cache/go-build
	GOCACHE=$(PWD)/.cache/go-build go build -o dist/hostshift ./cmd/hostshift

test: test-node test-go

test-node:
	npm test

test-go:
	mkdir -p .cache/go-build
	GOCACHE=$(PWD)/.cache/go-build go test ./...

test-integration-docker:
	bash tests/integration/docker/run-matrix.sh

docker-pull-fixtures:
	node tests/integration/docker/run-matrix.mjs --pull-images

test-e2e-vm:
	bash tests/e2e/vm/run-vm-e2e.sh

checksum: build
	cd dist && shasum -a 256 hostshift > checksums.txt

sbom:
	mkdir -p dist
	node scripts/make-sbom.mjs dist/hostshift.sbom.spdx.json

release-snapshot:
	if command -v goreleaser >/dev/null 2>&1; then \
		goreleaser release --snapshot --clean; \
	else \
		$(MAKE) build checksum sbom; \
	fi
