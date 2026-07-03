# HostShift

HostShift is a read-only-source server migration tool for Ubuntu and Debian web workloads.

The core rule is simple: the source server is an observation endpoint. HostShift may read inventory, stream data to stdout, and run typed read-only exports, but it must not write files, install packages, manage services, alter firewall rules, add keys, create snapshots, or place applications into maintenance mode on the source.

## Status

This repository is in the v0.7 validation milestone. The existing Node CLI remains as the v0.2 behavior reference, while the Go `hostshift` CLI is the primary implementation. The Docker migration matrix is executable and includes real SSH-driven source/target containers. The VM e2e layer boots real Lima instances, captures source snapshots, runs HostShift `discover`, `plan`, `prepare`, `sync`, and `verify` over SSH, validates workload parity, reboots the target, verifies persistence, and checks that the source snapshot did not change.

Release packaging is scaffolded with GoReleaser, local checksum generation, and a generated SPDX SBOM. Cloud and DNS automation remain intentionally outside the core.

## Quick Start

Install a release binary with `docs/install.md`, or run from source during development:

```bash
go run ./cmd/hostshift version
go run ./cmd/hostshift doctor --source old-server --target new-server --json
go run ./cmd/hostshift discover --source old-server --name example --profile example.profile.yaml --json
go run ./cmd/hostshift plan --profile examples/profile.yaml --target new-server --json
go run ./cmd/hostshift prepare --profile examples/profile.yaml --target new-server --json
go run ./cmd/hostshift sync --profile examples/profile.yaml --target new-server --json
go run ./cmd/hostshift verify --profile examples/profile.yaml --target new-server --json
go run ./cmd/hostshift profile migrate --input examples/profile.yaml --output /tmp/profile.v2.json
```

With an installed binary, replace `go run ./cmd/hostshift` with `hostshift`.

`prepare`, `sync`, and `verify` default to dry-run mode and write resumable state. Add `--apply` only after reviewing blockers and actions.

Sync plans may include `streams`, which pipe a validated source read command into a validated target write command. Examples include `tar --create -> tar --extract`, `docker image save -> docker image load`, `mysqldump -> mysql`, and `pg_dump -> pg_restore`.

See `examples/profile.v2.yaml` and `examples/web-stack-v2.yaml` for v2 YAML workload metadata and secret environment references.

Verify checks currently include HTTP status checks with an optional Host header, Laravel database connectivity through a reviewed target-side container command, target-side `fileExists` / `fileContains` assertions, `mysqlScalar` / `postgresScalar` read-only database assertions, `serviceActive` systemd checks, `ufwRule` and `nftRule` firewall rule checks, and `nginxConfig` target reload validation after config sync.

Target package preparation is capability based. HostShift derives requirements from workloads and checks, for example `rsync`, `tar`, `curl`, `docker-runtime`, `docker-compose`, `mysql-client`, and `postgresql-client`, then maps them through the Ubuntu/Debian platform adapter before emitting target-side install actions. Unknown target platforms block apply instead of guessing package names.

First-install target configuration can also be modeled in profile v2. HostShift validates and plans UFW rules, OpenSSH keepalive drop-ins, and MySQL bind-address drop-ins as target-only prepare actions with rollback metadata. These settings are never applied to the source.

## Development

```bash
make test
make test-node
make test-go
make build
make test-e2e-vm
make docker-pull-fixtures
make release-snapshot
```

## Documentation Website

The documentation website is built with Astro Starlight under `docs-site/`.

```bash
cd docs-site
npm install
npm run dev
```

Or run it with Docker Compose:

```bash
docker compose -f docs-site/compose.yml up --build
```

Open `http://localhost:4321`.

## Codex Plugin

HostShift also ships as a Codex plugin package under `plugins/hostshift`. The plugin does not replace the CLI; it gives Codex the migration workflow, source-safety rules, and operator guidance while the deterministic `hostshift` binary performs discovery, planning, transfer, and verification.

This repository includes a repo marketplace at `.agents/plugins/marketplace.json`. Add it to Codex, then install the plugin from the `hostshift` marketplace:

```bash
codex plugin marketplace add https://github.com/oguzhankrcb/HostShift.git
codex plugin add hostshift@hostshift
```

For local development from a checkout, add the repository root instead:

```bash
codex plugin marketplace add .
codex plugin add hostshift@hostshift
```

Start a new Codex thread after installing or updating the plugin so the bundled `migrate-server` skill is loaded.

See `docs/validation.md` for the release gates. In short, quick unit checks are not enough for a release: the real Docker matrix and the VM apply gate must also pass with the current `dist/hostshift` binary.

Docker matrix tests are scaffolded under `tests/integration/docker` and will run with:

```bash
make test-integration-docker
```

Set `HOSTSHIFT_RUN_DOCKER_MATRIX=1` to build the fixture pairs, verify source immutability markers, establish temporary SSH aliases, run `hostshift discover` over SSH, execute dry-run `plan`/`prepare`/`sync`/`verify`, then run real `sync --apply` and `verify --apply` smoke paths for each matrix pair with file-copy, MySQL restore, PostgreSQL restore, HTTP health, Laravel-style DB connectivity, and checksum assertions.

Real Docker matrix mode requires a running Docker daemon and access to the base images listed in `tests/integration/docker/run-matrix.mjs`. Docker commands have explicit timeouts so a daemon-side registry or proxy stall fails visibly instead of hanging the matrix.

Useful Docker matrix diagnostics:

```bash
node tests/integration/docker/run-matrix.mjs --list-images
make docker-pull-fixtures
HOSTSHIFT_DOCKER_PULL_TIMEOUT_MS=60000 HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker
```

Real mode pre-pulls required base images before building fixtures unless `HOSTSHIFT_DOCKER_SKIP_PREPULL=1` is set. This makes Docker daemon registry/proxy failures visible before the matrix starts mutating test containers.

VM e2e scaffolding lives under `tests/e2e/vm`:

```bash
make test-e2e-vm
HOSTSHIFT_RUN_VM_E2E=1 make test-e2e-vm
make build
HOSTSHIFT_RUN_VM_E2E=1 bash tests/e2e/vm/run-vm-e2e.sh --pair ubuntu22->debian12 --apply
```

The default run is a dry-run that validates the Ubuntu/Debian matrix, renders per-pair VM workspaces, emits provider-specific Lima templates plus command manifests, and keeps the source-safety model explicit in every generated artifact. `HOSTSHIFT_RUN_VM_E2E=1` adds provider preflight and validates that the required binaries exist locally. Adding `--apply` boots the rendered source and target VMs through Lima, assembles a temporary SSH config from `limactl show-ssh`, captures a source-side file snapshot, runs HostShift `discover`, `plan`, `prepare`, `sync`, and `verify` against the live aliases, verifies file parity, Nginx HTTP health, MySQL row/checksum parity, PostgreSQL row/checksum parity, systemd service state, and planned UFW/nftables rule state, restarts the target VM and runs `verify` again for boot persistence, checks that the source snapshot did not change, then tears the instances down unless `HOSTSHIFT_VM_KEEP_INSTANCES=1` is set. The VM runner uses `dist/hostshift` when it exists and falls back to the Node reference CLI only when the Go binary has not been built.

## Release

```bash
make release-snapshot
make checksum
make sbom
```

`make release-snapshot` uses GoReleaser when it is installed. Without GoReleaser it still builds `dist/hostshift`, writes `dist/checksums.txt`, and generates `dist/hostshift.sbom.spdx.json` from the Go module graph. Tagged GitHub releases use `.github/workflows/release.yml`.

Release candidates must satisfy the gates in `docs/validation.md`, including source immutability, the cross-distro Docker matrix, the real VM apply gate, checksums, and SBOM output.

Before publishing a public tag, also complete the checklist in `docs/release.md`: hosted CI candidate, self-hosted or local VM apply, clean working tree, and signed checksum verification.

## Supported Targets

The first support matrix focuses on Ubuntu 22.04/24.04/26.04 LTS, Ubuntu 25.10 as an interim release while supported, and Debian 12/13. EOL targets are blocked by default; EOL sources can still be read when the migration remains source-safe.

## License

Apache-2.0. See `LICENSE`.
